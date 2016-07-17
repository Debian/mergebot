package loggedexec_test

import (
	"fmt"

	"github.com/Debian/mergebot/loggedexec"
)

func ExampleCommand() {
	cmd := loggedexec.Command("ls", "/tmp/nonexistant")
	cmd.Env = []string{"LANG=C"}
	if err := cmd.Run(); err != nil {
		fmt.Println(err)
	}
	// Output: Running "ls /tmp/nonexistant": exit status 2
	// See "/tmp/005-ls.invocation.log" for invocation details.
	// See "/tmp/005-ls.stdoutstderr.log" for full stdout/stderr.
	// First stdout/stderr line: "ls: cannot access /tmp/nonexistant: No such file or directory"
}
