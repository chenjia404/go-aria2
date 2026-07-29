package httpdl

import "strings"

func mirrorURLs(st *state) []string {
	if st == nil {
		return nil
	}
	if len(st.sourceURLs) > 0 {
		return append([]string(nil), st.sourceURLs...)
	}
	if strings.TrimSpace(st.sourceURL) != "" {
		return []string{st.sourceURL}
	}
	return nil
}

func (st *state) setActiveMirror(index int) {
	urls := mirrorURLs(st)
	if index < 0 || index >= len(urls) {
		return
	}
	st.sourceIndex = index
	st.sourceURL = urls[index]
}
