package main

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/apex/log"
	"github.com/konveyor-ecosystem/kantra/cmd"
)

func main() {
	cmd.Execute()

	runAnalyzer := cmd.RunAnalyzer()
	options, err := cmd.BuildOptions()
	if err != nil {
		log.Errorf("err: %v", err)
	}

	if runAnalyzer {
		// analyzerCmd := exec.Command("/usr/local/bin/konveyor-analyzer", options)
		analyzerCmd := exec.Command("./konveyor-analyzer", options)
		var out bytes.Buffer
		var stderr bytes.Buffer
		analyzerCmd.Stdout = &out
		analyzerCmd.Stderr = &stderr
		err := analyzerCmd.Run()
		if err != nil {
			fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		}
		fmt.Println(fmt.Sprintf("Result: %v", out.String()))
	}
}
