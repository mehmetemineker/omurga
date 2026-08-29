package remote

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Result struct {
	Code    int
	Command []string
}

func SSHArguments(profile Profile, arguments []string, tty bool) []string {
	ssh := []string{}
	if tty {
		ssh = append(ssh, "-t")
	}
	if profile.Port > 0 {
		ssh = append(ssh, "-p", strconv.Itoa(profile.Port))
	}
	if profile.Identity != "" {
		ssh = append(ssh, "-i", profile.Identity)
	}
	target := profile.Address
	if profile.User != "" {
		target = profile.User + "@" + target
	}
	ssh = append(ssh, "--", target)
	remoteCommand := []string{}
	if profile.Sudo {
		remoteCommand = append(remoteCommand, "sudo", "-n")
	}
	binary := profile.OmurgaPath
	if binary == "" {
		binary = "omurga"
	}
	remoteCommand = append(remoteCommand, binary)
	remoteCommand = append(remoteCommand, arguments...)
	quoted := make([]string, len(remoteCommand))
	for index, value := range remoteCommand {
		quoted[index] = shellQuote(value)
	}
	return append(ssh, strings.Join(quoted, " "))
}

func Execute(ctx context.Context, profile Profile, arguments []string, tty bool, stdin io.Reader, stdout, stderr io.Writer) (Result, error) {
	sshArguments := SSHArguments(profile, arguments, tty)
	command := exec.CommandContext(ctx, "ssh", sshArguments...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return Result{Code: exitError.ExitCode(), Command: append([]string{"ssh"}, sshArguments...)}, nil
		}
		return Result{}, fmt.Errorf("could not execute SSH command: %w", err)
	}
	return Result{Code: 0, Command: append([]string{"ssh"}, sshArguments...)}, nil
}

func RemoveHostFlag(arguments []string) []string {
	result := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--host" {
			index++
			continue
		}
		if strings.HasPrefix(argument, "--host=") {
			continue
		}
		result = append(result, argument)
	}
	return result
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
