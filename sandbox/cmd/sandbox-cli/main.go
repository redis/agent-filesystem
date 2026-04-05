// Command sandbox-cli is a CLI for the sandbox server.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

var baseURL string

func main() {
	flag.StringVar(&baseURL, "url", "http://localhost:8090", "Sandbox server URL")
	flag.Parse()

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

	var err error
	switch cmd {
	case "launch", "run":
		err = cmdLaunch(args)
	case "read", "output":
		err = cmdRead(args)
	case "write", "input":
		err = cmdWrite(args)
	case "kill", "stop":
		err = cmdKill(args)
	case "list", "ps":
		err = cmdList()
	case "wait":
		err = cmdWait(args)
	default:
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`sandbox-cli - Sandbox command line interface

Usage:
  sandbox-cli [flags] <command> [args...]

Commands:
  launch <command>     Launch a process (use -w to wait)
  read <id>            Read process output
  write <id> <input>   Write to process stdin
  kill <id>            Kill a process
  list                 List all processes
  wait <id>            Wait for process to complete

Flags:`)
	flag.PrintDefaults()
}

func cmdLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ExitOnError)
	wait := fs.Bool("w", false, "Wait for completion")
	cwd := fs.String("d", "", "Working directory")
	timeout := fs.Int("t", 0, "Timeout in seconds")
	keepStdin := fs.Bool("i", false, "Keep stdin open")
	fs.Parse(args)

	if fs.NArg() < 1 {
		return fmt.Errorf("command required")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"command":         fs.Arg(0),
		"cwd":             *cwd,
		"timeout_secs":    *timeout,
		"wait":            *wait,
		"keep_stdin_open": *keepStdin,
	})

	resp, err := http.Post(baseURL+"/processes", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return printJSON(resp.Body)
}

func cmdRead(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("process ID required")
	}
	resp, err := http.Get(baseURL + "/processes/" + args[0])
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp.Body)
}

func cmdWrite(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("process ID and input required")
	}
	body, _ := json.Marshal(map[string]string{"input": args[1]})
	resp, err := http.Post(baseURL+"/processes/"+args[0]+"/write", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp.Body)
}

func cmdKill(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("process ID required")
	}
	req, _ := http.NewRequest("DELETE", baseURL+"/processes/"+args[0], nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp.Body)
}

func cmdList() error {
	resp, err := http.Get(baseURL + "/processes")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp.Body)
}

func cmdWait(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("process ID required")
	}
	resp, err := http.Post(baseURL+"/processes/"+args[0]+"/wait", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printJSON(resp.Body)
}

func printJSON(r io.Reader) error {
	var data interface{}
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return err
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(out))
	return nil
}

