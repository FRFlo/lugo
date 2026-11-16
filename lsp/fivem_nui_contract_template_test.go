package lsp

import (
	"reflect"
	"testing"
)

func TestNUIJSNamesParentResourceTemplate(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "common fetch template",
			src:  "fetch(`https://${GetParentResourceName()}/research_cb`, ...);",
			want: []string{"research_cb"},
		},
		{
			name: "template with interpolation spacing",
			src:  "fetch(`https://${  GetParentResourceName()  }/research_cb`);",
			want: []string{"research_cb"},
		},
		{
			name: "malformed or dynamic resource expression",
			src:  "fetch(`https://${getName()}/research_cb`); fetch(`https://${GetParentResourceName()}/${suffix}`);",
		},
		{
			name: "existing callback spellings remain recognized",
			src:  `fetch(GetParentResourceName()/old_cb'); fetch("https://resource.example/old_http_cb", {});`,
			want: []string{"old_cb", "old_http_cb"},
		},
		{
			name: "unicode case fold remains recognized",
			src:  "fetch(`https://${GetParentReſourceName()}/unicode_cb`);",
			want: []string{"unicode_cb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := []byte(tt.src)
			got := nuiJSNames(src, false)
			var names []string
			for _, name := range got {
				names = append(names, name.name)
				if name.offset < 0 || name.offset+len(name.name) > len(src) || string(src[name.offset:name.offset+len(name.name)]) != name.name {
					t.Fatalf("match offset %d does not select %q in source", name.offset, name.name)
				}
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Fatalf("nuiJSNames() names = %v, want %v", names, tt.want)
			}
		})
	}
}
