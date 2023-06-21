package app

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

	"github.com/sourcegraph/jsonrpc2"

	"github.com/lighttiger2505/sqls/internal/config"
	"github.com/lighttiger2505/sqls/internal/handler"
	"github.com/lighttiger2505/sqls/internal/lsp"
)

var client *jsonrpc2.Conn

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

func Serve(logfile, configFile string, trace bool) *jsonrpc2.Conn {
	// Initialize log writer
	var logWriter io.Writer
	if logfile != "" {
		f, err := os.OpenFile(logfile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		os.Stderr = f
		logWriter = io.MultiWriter(os.Stderr, f)
	} else {
		logWriter = io.MultiWriter(os.Stderr)
	}
	log.SetOutput(logWriter)
	// Set connect option
	var connOpt []jsonrpc2.ConnOpt
	if trace {
		connOpt = append(connOpt, jsonrpc2.LogMessages(log.New(logWriter, "", 0)))
	}

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
			fmt.Printf("cannot read specified config, %s", err.Error())
		}
		server.SpecificFileCfg = cfg
	} else {
		// Load default config
		cfg, err := config.GetDefaultConfig()
		if err != nil && !errors.Is(config.ErrNotFoundConfig, err) {
			fmt.Printf("cannot read default config, %s", err.Error())
		}
		server.DefaultFileCfg = cfg
	}

	// Start language server
	a, b := inMemoryPeerConns()
	//defer a.Close()
	//defer b.Close()

	jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(a, jsonrpc2.VSCodeObjectCodec{}),
		h,
		connOpt...,
	)

	client = jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(b, jsonrpc2.VSCodeObjectCodec{}),
		noopHandler{},
	)

	//defer serverConn.Close()
	//defer client.Close()

	var resp interface{}
	// Send an LSP initialize request

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
	}

	statement_1 := ""
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
	}

	return client
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
