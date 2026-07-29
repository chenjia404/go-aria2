package aria2

import (
	"context"
	"encoding/json"

	"github.com/chenjia404/go-aria2/internal/protocol/ed2k"
)

type ed2kGatewayNative struct {
	gateway *ed2k.HTTPGateway
}

// NewED2KNativeFromGateway 将 ED2K HTTP 网关适配为 native.* RPC 后端。
func NewED2KNativeFromGateway(gateway *ed2k.HTTPGateway) ED2KNativeAPI {
	if gateway == nil {
		return nil
	}
	return &ed2kGatewayNative{gateway: gateway}
}

func (a *ed2kGatewayNative) Servers(ctx context.Context) ([]map[string]any, error) {
	servers, err := a.gateway.Servers(ctx)
	if err != nil {
		return nil, err
	}
	return dtoSliceToMaps(servers)
}

func (a *ed2kGatewayNative) ConnectServer(ctx context.Context, addr string) error {
	return a.gateway.ConnectServer(ctx, addr)
}

func (a *ed2kGatewayNative) KadStatus(ctx context.Context) (map[string]any, error) {
	status, err := a.gateway.DHTStatus(ctx)
	if err != nil {
		return nil, err
	}
	return dtoToMap(status)
}

func (a *ed2kGatewayNative) ConnectKad(ctx context.Context, nodes []string) error {
	return a.gateway.AddDHTBootstrapNodes(ctx, nodes)
}

func dtoToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func dtoSliceToMaps(v any) ([]map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
