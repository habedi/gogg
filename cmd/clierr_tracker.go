package cmd

import "github.com/habedi/gogg/pkg/clierr"

var lastCliErr *clierr.Error

func setLastCliErr(e *clierr.Error) { lastCliErr = e }
func getLastCliErr() *clierr.Error  { return lastCliErr }
