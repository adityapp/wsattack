package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var ErrEndofArgs = fmt.Errorf("end of args")
var ErrNotaFlag = fmt.Errorf("arg is not a flag")

type allArgs struct {
	SeederDir     []os.DirEntry
	SeederDirFlag bool

	HelpFlag bool

	FileSeedFlag bool
	FileSeed     string

	WsTargetFlag bool
	WsTarget     string

	WsAuthHeaderFlag bool
	WsAuthHeader     map[string]string

	AtkTimes int64

	Err error
}

func (a *allArgs) Parse(args []string) {
	a.seekFlag(1, args, false)
}

func (a *allArgs) seekFlag(idx int, args []string, chain bool) (string, error) {
	if len(args) == 0 {
		return "", ErrEndofArgs
	}

	if idx > len(args)-1 {
		return "", ErrEndofArgs
	}

	idxForward := idx
	switch inspected := args[idx]; inspected {
	case "--seeder-dir":
		var dirPath string
		var err error
		idxForward++
		dirPath, err = a.seekFlag(idxForward, args, true)
		if errors.Is(err, ErrEndofArgs) {
			a.Err = fmt.Errorf("no dir path provided for flag --seeder-dir")
			break
		}

		seederDir, err := os.ReadDir(dirPath)
		if err != nil {
			a.Err = fmt.Errorf("flag --seeder-dir %w", err)
			break
		}

		var count int
		for _, aFile := range seederDir {
			if !aFile.IsDir() {
				count++
			}
		}

		if count == 0 {
			a.Err = fmt.Errorf("flag --seeder-dir, no files under %s", dirPath)
			break
		}

		a.SeederDirFlag = true
	case "--help":
		a.HelpFlag = true
	case "--ws-target":
		idxForward++
		var wsTarget string
		var err error
		wsTarget, err = a.seekFlag(idxForward, args, true)
		if errors.Is(err, ErrEndofArgs) {
			a.Err = fmt.Errorf("flag --ws-target need a filename")
			break
		}

		a.WsTarget = wsTarget
		a.WsTargetFlag = true
	case "--times":
		idxForward++
		timeNum, err := a.seekFlag(idxForward, args, true)
		if errors.Is(err, ErrEndofArgs) {
			a.Err = fmt.Errorf("flag --times need to be integer")
			break
		}

		a.AtkTimes, err = strconv.ParseInt(timeNum, 10, 64)
		if err != nil {
			a.Err = fmt.Errorf("flag --times need to be integer")
			break
		}
	case "--ws-auth-header":
		idxForward++
		wsAuth, err := a.seekFlag(idxForward, args, true)
		if errors.Is(err, ErrEndofArgs) {
			a.Err = fmt.Errorf("flag --ws-auth-header need an auth string")
			break
		}

		// split `:`
		wsh := strings.Split(wsAuth, ":")
		if len(wsh) < 2 {
			a.Err = fmt.Errorf("flag --ws-auth-header must follow format 'header: value'")
			break
		}
		a.WsAuthHeader = make(map[string]string)
		a.WsAuthHeader[strings.TrimSpace(wsh[0])] = strings.TrimSpace(wsh[1])
		a.WsAuthHeaderFlag = true
	default:
		if chain {
			return inspected, ErrNotaFlag
		}

		a.FileSeedFlag = true

		var err error
		fileTarget, err := os.Open(inspected)
		if err != nil {
			a.Err = fmt.Errorf("%w", err)
		}

		a.FileSeed = inspected
		fileTarget.Close()
	}
	idxForward++

	return a.seekFlag(idxForward, args, false)
}

func main() {
	var parseArgs allArgs
	parseArgs.Parse(os.Args)

	if parseArgs.Err != nil {
		fmt.Fprintf(os.Stderr, "%v", parseArgs.Err)
	}

	switch {
	case parseArgs.HelpFlag:
	case parseArgs.SeederDirFlag:
		fallthrough
	case parseArgs.FileSeedFlag:
		fallthrough
	case parseArgs.WsTargetFlag:
	}

	fmt.Fprintf(os.Stdout, "flag debug %v\n", parseArgs)

	_, err := NewWsAtk(parseArgs.SeederDir, parseArgs.FileSeed, parseArgs.WsTarget, parseArgs.WsAuthHeader, parseArgs.AtkTimes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init atttack %v", err)
		os.Exit(1)
	}
}
