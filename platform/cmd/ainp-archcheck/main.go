package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yzxingtu2026/ainp/platform/framework/modulecheck"
)

func main() {
	profile := flag.String("profile", "", "architecture profile: ainp or downstream")
	modulesRoot := flag.String("modules-root", "", "path to the module packages root")
	testsRoot := flag.String("tests-root", "", "path to the centralized tests root")
	modulePath := flag.String("module-path", "", "Go module path; defaults to the nearest go.mod")
	flag.Parse()

	violations, err := modulecheck.Check(modulecheck.Config{
		Profile:     modulecheck.Profile(*profile),
		ModulesRoot: *modulesRoot,
		TestsRoot:   *testsRoot,
		ModulePath:  *modulePath,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(violations) == 0 {
		fmt.Println("module architecture check passed")
		return
	}
	for _, violation := range violations {
		fmt.Fprintln(os.Stderr, violation.String())
	}
	os.Exit(1)
}
