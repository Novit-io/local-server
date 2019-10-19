package main

import (
	"testing"

	yaml "gopkg.in/yaml.v2"
)

func TestMerge(t *testing.T) {
	if v := genericMerge("a", "b"); v != "b" {
		t.Errorf("got %q", v)
	}

    if v := unparse(genericMerge(parse(`
a: t
b: t
m:
  a1: t
  b1: t
`), parse(`
a: s
c: s
m:
  a1: s
  c1: s
`))); "\n"+v != `
a: s
b: t
c: s
m:
  a1: s
  b1: t
  c1: s
` {
    t.Errorf("got\n%s", v)
}
}

func parse(s string) (r interface{}) {
	r = map[string]interface{}{}
	err := yaml.Unmarshal([]byte(s), r)
	if err != nil {
		panic(err)
	}
	return
}

func unparse(v interface{}) (s string) {
	ba, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(ba)
}
