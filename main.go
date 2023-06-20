package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	"github.com/urfave/cli/v2"

	"github.com/lighttiger2505/sqls/internal/config"
	"github.com/lighttiger2505/sqls/internal/handler"
	"github.com/lighttiger2505/sqls/internal/lsp"
)

// builtin variables. see Makefile
var (
	version  string
	revision string
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func realMain() error {
	app := &cli.App{
		Name:    "sqls",
		Version: fmt.Sprintf("Version:%s, Revision:%s\n", version, revision),
		Usage:   "An implementation of the Language Server Protocol for SQL.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log",
				Aliases: []string{"l"},
				Usage:   "Also log to this file. (in addition to stderr)",
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Specifies an alternative per-user configuration file. If a configuration file is given on the command line, the workspace option (initializationOptions) will be ignored.",
			},
			&cli.BoolFlag{
				Name:    "trace",
				Aliases: []string{"t"},
				Usage:   "Print all requests and responses.",
			},
		},
		Commands: cli.Commands{
			{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "edit config",
				Action: func(c *cli.Context) error {
					editorEnv := os.Getenv("EDITOR")
					if editorEnv == "" {
						editorEnv = "vim"
					}
					return OpenEditor(editorEnv, config.YamlConfigPath)
				},
			},
		},
		Action: func(c *cli.Context) error {
			return serve(c)
		},
	}
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "Print version.",
	}
	cli.HelpFlag = &cli.BoolFlag{
		Name:    "help",
		Aliases: []string{"h"},
		Usage:   "Print help.",
	}

	err := app.Run(os.Args)
	if err != nil {
		return err
	}

	return nil
}

type noopHandler struct{}

func (noopHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {}

type pipeReadWriteCloser struct {
	*io.PipeReader
	*io.PipeWriter
}

func (c *pipeReadWriteCloser) Close() error {
	err1 := c.PipeReader.Close()
	err2 := c.PipeWriter.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func inMemoryPeerConns() (io.ReadWriteCloser, io.ReadWriteCloser) {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	return &pipeReadWriteCloser{sr, sw}, &pipeReadWriteCloser{cr, cw}
}

func serve(c *cli.Context) error {
	os.Stdout.Write([]byte("hello world server\n"))
	logfile := c.String("log")
	configFile := c.String("config")
	trace := c.Bool("trace")

	// Initialize log writer
	var logWriter io.Writer
	if logfile != "" {
		f, err := os.OpenFile(logfile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		logWriter = io.MultiWriter(os.Stderr, f)
	} else {
		logWriter = io.MultiWriter(os.Stderr)
	}
	log.SetOutput(logWriter)

	// Initialize language server
	server := handler.NewServer()
	defer func() {
		if err := server.Stop(); err != nil {
			log.Println(err)
		}
	}()
	h := jsonrpc2.HandlerWithError(server.Handle)

	// Load specific config
	if configFile != "" {
		cfg, err := config.GetConfig(configFile)
		if err != nil {
			return fmt.Errorf("cannot read specified config, %w", err)
		}
		server.SpecificFileCfg = cfg
	} else {
		// Load default config
		cfg, err := config.GetDefaultConfig()
		if err != nil && !errors.Is(config.ErrNotFoundConfig, err) {
			return fmt.Errorf("cannot read default config, %w", err)
		}
		server.DefaultFileCfg = cfg
	}

	// Set connect option
	var connOpt []jsonrpc2.ConnOpt
	if trace {
		connOpt = append(connOpt, jsonrpc2.LogMessages(log.New(logWriter, "", 0)))
	}

	// Start language server
	fmt.Println("sqls: reading on stdin, writing on stdout")
	//stdio := stdrwc{}

	a, b := inMemoryPeerConns()
	defer a.Close()
	defer b.Close()

	/* 	go func() {
		svConn := jsonrpc2.NewConn(
			context.Background(),
			jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}),
			h,
			connOpt...,
		)

		<-svConn.DisconnectNotify()
		log.Println("sqls: connections closed")
	}() */

	serverConn := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}),
		h,
		connOpt...,
	)

	client := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}),
		noopHandler{},
	)

	defer serverConn.Close()
	defer client.Close()

	var resp interface{}

	// Send an LSP initialize request

	time.Sleep(2000 * time.Millisecond)
	fmt.Printf("call request \n")

	err := client.Call(
		context.Background(),
		"initialize",
		lsp.InitializeParams{
			ProcessID: 1,
			RootURI:   "file:///Users/gdemorais/qdev/temp/sqls/client",
			Trace:     "verbose",
		},
		&resp)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	} else {
		fmt.Println("response init: ", resp)
	}

	statement_1 := "select * fr"
	statement_2 := "select * from u"
	err = client.Call(
		context.Background(),
		"textDocument/didOpen",
		lsp.DidOpenTextDocumentParams{
			TextDocument: lsp.TextDocumentItem{
				URI:        "test.sql",
				LanguageID: "sql",
				Version:    1,
				Text:       statement_1,
			},
		},
		&resp)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	} else {
		fmt.Println("response didOpen: ", resp)
	}

	completionParams := lsp.CompletionParams{TextDocumentPositionParams: lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{
			URI: "test.sql",
		},
		Position: lsp.Position{
			Line:      0,
			Character: len(statement_1),
		},
	}}

	err = client.Call(context.Background(), "textDocument/completion", completionParams, &resp)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	} else {
		fmt.Println("response completion: ", resp)
	}

	didchangeParams := lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			Version: 2,
			URI:     "test.sql",
		},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{
			{Text: statement_2},
		},
	}

	err = client.Call(context.Background(), "textDocument/didChange", didchangeParams, &resp)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	} else {
		fmt.Println("response didChange: ", resp)
	}

	time.Sleep(2000 * time.Millisecond)
	// Update cursor position
	completionParams.TextDocumentPositionParams.Position.Character = len(statement_2)

	err = client.Call(context.Background(), "textDocument/completion", completionParams, &resp)

	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	} else {
		fmt.Println("response completion: ", resp)
	}

	return nil
}

type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	//fmt.Println("server read")
	//fmt.Println(p)
	//return -1, nil
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	//fmt.Println("server write")
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}

func OpenEditor(program string, args ...string) error {
	cmdargs := strings.Join(args, " ")
	command := program + " " + cmdargs

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getContentChanges(oldStr, newStr string) []lsp.TextDocumentContentChangeEvent {
	var changes []lsp.TextDocumentContentChangeEvent

	// Find the common prefix and suffix of the two strings.
	prefix := 0
	for prefix < len(oldStr) && prefix < len(newStr) && oldStr[prefix] == newStr[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldStr)-prefix && suffix < len(newStr)-prefix && oldStr[len(oldStr)-suffix-1] == newStr[len(newStr)-suffix-1] {
		suffix++
	}

	// Calculate the range of changed text.
	start := lsp.Position{Line: 0, Character: prefix}
	end := lsp.Position{Line: 0, Character: len(newStr) - suffix}

	// Generate a content change event based on the modified text.
	changeEvent := lsp.TextDocumentContentChangeEvent{
		Range: lsp.Range{
			Start: start,
			End:   end,
		},
		Text: strings.TrimPrefix(strings.TrimSuffix(newStr, string(oldStr[len(oldStr)-suffix:])), oldStr[:prefix]),
	}

	changes = append(changes, changeEvent)
	return changes
}
