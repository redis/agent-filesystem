package qmd

import "testing"

func TestParseDSLFilters(t *testing.T) {
	p, err := ParseDSL(`"disk full" AND retry path:/logs/ type:file size>10 size<999 mtime>100`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.PathPrefix != "/logs/" {
		t.Fatalf("path = %q", p.PathPrefix)
	}
	if p.TypeFilter != "file" {
		t.Fatalf("type = %q", p.TypeFilter)
	}
	if p.MinSize == nil || *p.MinSize != 10 {
		t.Fatalf("min size = %v", p.MinSize)
	}
	if p.MaxSize == nil || *p.MaxSize != 999 {
		t.Fatalf("max size = %v", p.MaxSize)
	}
	if p.MinMtimeMS == nil || *p.MinMtimeMS != 100 {
		t.Fatalf("min mtime = %v", p.MinMtimeMS)
	}
	if p.TextQuery != `"disk full" AND retry` {
		t.Fatalf("text query = %q", p.TextQuery)
	}
}

func TestBuildFTQuery(t *testing.T) {
	p, err := ParseDSL(`error OR panic path:/app/`)
	if err != nil {
		t.Fatal(err)
	}
	q := BuildFTQuery(p)
	want := `@type:{file} @path_ancestors:{\/app} error OR panic`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}
