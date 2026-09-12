package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"exchange_sim/sv1dlock"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sv1dlock", flag.ContinueOnError)
	flags.SetOutput(stderr)
	lockPath := flags.String("path", "", "absolute namespace lock path")
	flags.Usage = func() {
		fmt.Fprintf(stderr, "usage: sv1dlock -path LOCK -- COMMAND [ARG...]\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	commandArgs := flags.Args()
	if *lockPath == "" || len(commandArgs) == 0 {
		flags.Usage()
		return 2
	}
	lock, err := sv1dlock.Acquire(*lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	defer lock.Close()
	command := exec.Command(commandArgs[0], commandArgs[1:]...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.ExtraFiles = []*os.File{lock.File()}
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok {
				if waitStatus.Exited() {
					return waitStatus.ExitStatus()
				}
				if waitStatus.Signaled() {
					return 128 + int(waitStatus.Signal())
				}
			}
		}
		fmt.Fprintf(stderr, "run locked command: %v\n", err)
		return 1
	}
	return 0
}
