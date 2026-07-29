package xmlrpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/chenjia404/go-aria2/internal/rpc/jsonrpc"
)

// Handler 与 JSON-RPC 共用同一 Invoke 接口。
type Handler = jsonrpc.Handler

// Options 控制 XML-RPC HTTP 行为。
type Options struct {
	MaxRequestSize int64
	AllowOriginAll bool
}

// Server 提供 aria2 兼容的 XML-RPC 入口。
type Server struct {
	handler Handler
	options Options
}

// NewServer 创建 XML-RPC Server。
func NewServer(handler Handler, options Options) *Server {
	return &Server{handler: handler, options: options}
}

// ServeHTTP 处理 XML-RPC methodCall。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.options.AllowOriginAll {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	bodyReader := io.Reader(r.Body)
	if s.options.MaxRequestSize > 0 {
		bodyReader = http.MaxBytesReader(w, r.Body, s.options.MaxRequestSize)
	}
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		writeFault(w, jsonrpc.CodeInvalidRequest, err.Error())
		return
	}

	method, params, err := decodeMethodCall(body)
	if err != nil {
		writeFault(w, jsonrpc.CodeParseError, err.Error())
		return
	}

	result, invokeErr := s.handler.Invoke(r.Context(), method, params)
	if invokeErr != nil {
		var rpcErr *jsonrpc.RPCError
		if errors.As(invokeErr, &rpcErr) {
			writeFault(w, rpcErr.Code, rpcErr.Message)
			return
		}
		writeFault(w, jsonrpc.CodeInternalError, invokeErr.Error())
		return
	}

	payload, err := encodeMethodResponse(result)
	if err != nil {
		writeFault(w, jsonrpc.CodeInternalError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write(payload)
}

func decodeMethodCall(body []byte) (string, []any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var methodName string
	var params []any
	inParams := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", nil, err
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "methodName":
				var name string
				if err := decoder.DecodeElement(&name, &elem); err != nil {
					return "", nil, err
				}
				methodName = strings.TrimSpace(name)
			case "params":
				inParams = true
			case "param":
				if !inParams {
					continue
				}
				value, err := decodeValue(decoder)
				if err != nil {
					return "", nil, err
				}
				params = append(params, value)
			}
		}
	}
	if methodName == "" {
		return "", nil, fmt.Errorf("methodName is required")
	}
	return methodName, params, nil
}

func decodeValue(decoder *xml.Decoder) (any, error) {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "value":
			return decodeValueContent(decoder)
		case "string":
			var s string
			if err := decoder.DecodeElement(&s, &start); err != nil {
				return nil, err
			}
			return s, nil
		case "int", "i4", "i8":
			var raw string
			if err := decoder.DecodeElement(&raw, &start); err != nil {
				return nil, err
			}
			n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				return nil, err
			}
			return float64(n), nil
		case "boolean":
			var raw string
			if err := decoder.DecodeElement(&raw, &start); err != nil {
				return nil, err
			}
			return raw == "1" || strings.EqualFold(raw, "true"), nil
		case "base64":
			var raw string
			if err := decoder.DecodeElement(&raw, &start); err != nil {
				return nil, err
			}
			return base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
		case "array":
			return decodeArray(decoder)
		case "struct":
			return decodeStruct(decoder)
		default:
			var fallback string
			_ = decoder.DecodeElement(&fallback, &start)
			return fallback, nil
		}
	}
}

func decodeValueContent(decoder *xml.Decoder) (any, error) {
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch elem := tok.(type) {
		case xml.CharData:
			text := strings.TrimSpace(string(elem))
			if text != "" {
				return text, nil
			}
		case xml.StartElement:
			switch elem.Name.Local {
			case "string":
				var s string
				if err := decoder.DecodeElement(&s, &elem); err != nil {
					return nil, err
				}
				return s, nil
			case "int", "i4", "i8":
				var raw string
				if err := decoder.DecodeElement(&raw, &elem); err != nil {
					return nil, err
				}
				n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil {
					return nil, err
				}
				return float64(n), nil
			case "boolean":
				var raw string
				if err := decoder.DecodeElement(&raw, &elem); err != nil {
					return nil, err
				}
				return raw == "1" || strings.EqualFold(raw, "true"), nil
			case "base64":
				var raw string
				if err := decoder.DecodeElement(&raw, &elem); err != nil {
					return nil, err
				}
				return base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
			case "array":
				return decodeArray(decoder)
			case "struct":
				return decodeStruct(decoder)
			default:
				var fallback string
				_ = decoder.DecodeElement(&fallback, &elem)
				return fallback, nil
			}
		case xml.EndElement:
			if elem.Name.Local == "value" {
				return "", nil
			}
		}
	}
}

func decodeArray(decoder *xml.Decoder) ([]any, error) {
	out := []any{}
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == "value" {
				item, err := decodeValueContent(decoder)
				if err != nil {
					return nil, err
				}
				out = append(out, item)
			}
		case xml.EndElement:
			if elem.Name.Local == "array" {
				return out, nil
			}
		}
	}
}

func decodeStruct(decoder *xml.Decoder) (map[string]any, error) {
	out := map[string]any{}
	var currentName string
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "name":
				var name string
				if err := decoder.DecodeElement(&name, &elem); err != nil {
					return nil, err
				}
				currentName = name
			case "value":
				value, err := decodeValueContent(decoder)
				if err != nil {
					return nil, err
				}
				if currentName != "" {
					out[currentName] = value
					currentName = ""
				}
			}
		case xml.EndElement:
			if elem.Name.Local == "struct" {
				return out, nil
			}
		}
	}
}

func encodeMethodResponse(result any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString("<methodResponse><params><param><value>")
	if err := encodeValue(&buf, result); err != nil {
		return nil, err
	}
	buf.WriteString("</value></param></params></methodResponse>")
	return buf.Bytes(), nil
}

func encodeValue(buf *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case nil:
		buf.WriteString("<string></string>")
	case string:
		fmt.Fprintf(buf, "<string>%s</string>", xmlEscape(v))
	case bool:
		if v {
			buf.WriteString("<boolean>1</boolean>")
		} else {
			buf.WriteString("<boolean>0</boolean>")
		}
	case int:
		fmt.Fprintf(buf, "<int>%d</int>", v)
	case int32:
		fmt.Fprintf(buf, "<int>%d</int>", v)
	case int64:
		fmt.Fprintf(buf, "<int>%d</int>", v)
	case float32:
		fmt.Fprintf(buf, "<int>%d</int>", int(v))
	case float64:
		if v == float64(int64(v)) {
			fmt.Fprintf(buf, "<int>%d</int>", int64(v))
		} else {
			fmt.Fprintf(buf, "<double>%g</double>", v)
		}
	case []byte:
		fmt.Fprintf(buf, "<base64>%s</base64>", base64.StdEncoding.EncodeToString(v))
	case []any:
		buf.WriteString("<array><data>")
		for _, item := range v {
			buf.WriteString("<value>")
			if err := encodeValue(buf, item); err != nil {
				return err
			}
			buf.WriteString("</value>")
		}
		buf.WriteString("</data></array>")
	case []string:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = item
		}
		return encodeValue(buf, items)
	case map[string]any:
		buf.WriteString("<struct>")
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sortStrings(keys)
		for _, key := range keys {
			buf.WriteString("<member><name>")
			buf.WriteString(xmlEscape(key))
			buf.WriteString("</name><value>")
			if err := encodeValue(buf, v[key]); err != nil {
				return err
			}
			buf.WriteString("</value></member>")
		}
		buf.WriteString("</struct>")
	case map[string]string:
		converted := make(map[string]any, len(v))
		for key, val := range v {
			converted[key] = val
		}
		return encodeValue(buf, converted)
	default:
		fmt.Fprintf(buf, "<string>%s</string>", xmlEscape(fmt.Sprint(v)))
	}
	return nil
}

func writeFault(w http.ResponseWriter, code int, message string) {
	payload := fmt.Sprintf(`%s<methodResponse><fault><value><struct>
<member><name>faultCode</name><value><int>%d</int></value></member>
<member><name>faultString</name><value><string>%s</string></value></member>
</struct></value></fault></methodResponse>`, xml.Header, code, xmlEscape(message))
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(payload))
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

// InvokeMethod 便于测试：直接解码并调用 handler。
func InvokeMethod(ctx context.Context, h Handler, body []byte) ([]byte, error) {
	method, params, err := decodeMethodCall(body)
	if err != nil {
		return nil, err
	}
	result, invokeErr := h.Invoke(ctx, method, params)
	if invokeErr != nil {
		return nil, invokeErr
	}
	return encodeMethodResponse(result)
}
