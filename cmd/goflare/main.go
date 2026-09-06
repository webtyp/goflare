package main

import (
	"flag"
	"fmt"
	"os"

	"webtyp.com/goflare"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println(goflare.Usage())
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "auth":
		fs := flag.NewFlagSet("auth", flag.ExitOnError)
		check := fs.Bool("check", false, "Verify token from environment")
		env := fs.String("env", ".env", "path to .env file")
		fs.Parse(args)
		if err := goflare.RunAuth(*env, os.Stdout, *check); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		env := fs.String("env", ".env", "path to .env file")
		fs.Parse(args)
		if err := goflare.RunBuild(*env, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case "size":
		fs := flag.NewFlagSet("size", flag.ExitOnError)
		env := fs.String("env", ".env", "path to .env file")
		fs.Parse(args)
		if err := goflare.RunSize(*env, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case "tinygo":
		if err := goflare.RunTinyGo(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case "deploy":
		fs := flag.NewFlagSet("deploy", flag.ExitOnError)
		env := fs.String("env", ".env", "path to .env file")
		fs.Parse(args)
		if err := goflare.RunDeploy(*env, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

	case "help", "-h", "--help":
		fmt.Println(goflare.Usage())

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		fmt.Println(goflare.Usage())
		os.Exit(1)
	}
}
