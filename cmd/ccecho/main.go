package main

import "os"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "--version", "version", "-v":
		runVersion()
		return
	case "proxy":
		runProxy(os.Args[2:])
	case "run":
		runRun(os.Args[2:])
	case "codex":
		runCodex(os.Args[2:])
	case "view":
		runView(os.Args[2:])
	case "clean":
		runClean(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}
