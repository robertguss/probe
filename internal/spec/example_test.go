package spec_test

import (
	"fmt"

	"github.com/robertguss/go-foundry-cli/internal/spec"
)

// ExampleValidate shows the pure write-free path from TOML bytes to a
// validated Project Specification (ipk.13).
func ExampleValidate() {
	raw, err := spec.Decode("foundry.toml", []byte(`
schema = 1
name = "demo-cli"
module = "github.com/example/demo-cli"
description = "example project"
archetype = "cli"
destination = "./demo-cli"
visibility = "private"
profiles = []
`))
	if err != nil {
		fmt.Println("decode:", err)
		return
	}
	vs, err := spec.Validate(raw)
	if err != nil {
		fmt.Println("validate:", err)
		return
	}
	fmt.Println(vs.Name())
	// Output:
	// demo-cli
}
