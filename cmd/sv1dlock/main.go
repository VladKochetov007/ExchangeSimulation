package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"exchange_sim/sv1dlock"
)

func main() {
	lockPath := flag.String("path", "", "absolute namespace lock path")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s -path LOCK -- COMMAND [ARG...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	commandArgs := flag.Args()
	if *lockPath == "" || len(commandArgs) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	lock, err := sv1dlock.Acquire(*lockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	defer lock.Close()
	command := exec.Command(commandArgs[0], commandArgs[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.ExtraFiles = []*os.File{lock.File()}
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitError.Sys().(syscall.WaitStatus); ok {
				if waitStatus.Exited() {
					os.Exit(waitStatus.ExitStatus())
				}
				if waitStatus.Signaled() {
					os.Exit(128 + int(waitStatus.Signal()))
				}
			}
		}
		fmt.Fprintf(os.Stderr, "run locked command: %v\n", err)
		os.Exit(1)
	}
}
