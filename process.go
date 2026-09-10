package main

import "os/exec"

// chromedp appends os.Environ after ModifyCmdFunc if cmd.Env is nonempty.
// env -i replaces its own process with Chrome and passes only the allowlist.
func isolateBrowserCommand(cmd *exec.Cmd) {
	original := cmd.Args
	cmd.Path = "/usr/bin/env"
	cmd.Args = append([]string{"env", "-i"}, browserEnvironment()...)
	cmd.Args = append(cmd.Args, original...)
	cmd.Env = []string{}
	configureProcess(cmd)
}
