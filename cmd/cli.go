package cmd

import (
	"fmt"
	"github.com/habedi/template-go-project/internal/helpers"
	"github.com/habedi/template-go-project/pkg/package1"
)

// Execute runs the CLI code.
func Execute() {
	fmt.Println("This is the CLI program talking. :)")
	package1.DoSomething()
	fmt.Println(helpers.Add(1, 2))
}
