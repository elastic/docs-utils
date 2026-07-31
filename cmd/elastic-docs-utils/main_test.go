package main

import "testing"

func TestParseUpdateComponents(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  updateComponents
	}{
		{
			name:  "all by default",
			value: "all",
			want:  updateComponents{skills: true, vale: true, docsBuilder: true},
		},
		{
			name:  "selected components",
			value: "skills,vale-rules,docs-builder",
			want:  updateComponents{skills: true, vale: true, docsBuilder: true},
		},
		{
			name:  "vale aliases the rules installer",
			value: "vale",
			want:  updateComponents{vale: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseUpdateComponents(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("components = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseUpdateComponentsRejectsUnknownComponent(t *testing.T) {
	if _, err := parseUpdateComponents("skills,unknown"); err == nil {
		t.Fatal("unknown component was accepted")
	}
}
