package main

import "os"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "proxy":
		runProxy(os.Args[2:])
	case "run":
		runClaude(os.Args[2:])
	case "show":
		runShow(os.Args[2:])
	case "clean":
		runClean(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}
