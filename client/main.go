package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

type stdrwc struct {
	stdin  *io.WriteCloser
	stdout *io.ReadCloser
}

/* func (stdio *stdrwc) Read(p []byte) (int, error) {
	fmt.Println("client Read")
	return os.Stdin.Read(p)
	//return (*stdio.stdout).Read(p)
}

func (stdio *stdrwc) Write(p []byte) (int, error) {
	fmt.Println("client Write")
	//stdrwc.stdin.Write(p)
	//return os.Stdout.Write(p)
	return os.Stdout.Write(p)
	//return (*stdio.stdin).Write(p)
}

func (stdio *stdrwc) Close() error {

	if err := os.Stdout.Close(); err != nil {
		return err
	}
	return os.Stdin.Close()
} */

func (stdio *stdrwc) Read(p []byte) (int, error) {
	//fmt.Println("client Read")
	var b []byte
	n, err := (*stdio.stdout).Read(b)
	os.Stdin.Write(b)

	//(*stdio.stdout).Read(p)

	//return (*stdio.stdout).Read(p)
	return n, err
}

func (stdio *stdrwc) Write(p []byte) (int, error) {
	fmt.Println("client Write")
	//stdrwc.stdin.Write(p)
	//return os.Stdout.Write(p)

	return (*stdio.stdin).Write(p)
}

func (stdio *stdrwc) Close() error {

	if err := (*stdio.stdout).Close(); err != nil {
		return err
	}
	return (*stdio.stdin).Close()
}

func main() {
	fmt.Println("client main")
	/* 	configBytes, err := ioutil.ReadFile("config.yml")
	   	if err != nil {
	   		fmt.Println("Error reading config file:", err)
	   		return
	   	}
	*/
	fmt.Println("!exec!")
	cmd := exec.Command("./sqls", "-config", "/Users/gdemorais/qdev/temp/sqls/client/config.yml")

	stdio := stdrwc{}

	// Spawn a new process to run the LSP server.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdio.stdin = &stdin
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("failed to get stdout pipe: %v", err)
	}
	stdio.stdout = &stdout

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start process: %v", err)
	}

	os.Stdout.Write([]byte("hello world client\n"))
	// Communicate with the LSP server via stdio.
	// Use stdin and stdout pipes to send and receive messages, respectively.
	go func() {
		// Read incoming messages from the server.
		_, err := io.Copy(os.Stdout, stdout)
		if err != nil {
			log.Fatalf("error while copying stdout: %v", err)
		}
	}()

	fmt.Println("second go routine")
	go func() {
		// Send messages to the server.
		_, err := io.Copy(stdin, os.Stdin)
		if err != nil {
			log.Fatalf("error while copying stdin: %v", err)
		}
	}()

	fmt.Println("first go routine")

	client := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(&stdio, jsonrpc2.VSCodeObjectCodec{}),
		nil,
		nil,
	)

	var resp interface{}
	//var resp2 interface{}

	// Send an LSP initialize request

	time.Sleep(2000 * time.Millisecond)
	fmt.Printf("call request \n")
	err = client.Call(context.Background(), "initialize", map[string]interface{}{
		"processId": 1,
		"rootUri":   "test.sql",
		"rootPath":  "",
	}, &resp)

	fmt.Printf("Returned call! \n")
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
	}

	/* 	if err := json.Unmarshal(resp, &resp2); err != nil {
		fmt.Printf("Error unmarshaling response: %v\n", err)
		return
	} */

	// Handle the response
	fmt.Printf("Received response: %v\n", resp)

	//stdinPipe.write(JSON.stringify(initializeRequest));

	// Wait for the process to exit.
	if err := cmd.Wait(); err != nil {
		log.Fatalf("process exited with error: %v", err)
	}
}
