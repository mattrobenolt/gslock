package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func newStorageClient(ctx context.Context) (*storage.Client, error) {
	o := []option.ClientOption{}
	keyfile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if keyfile != "" {
		o = append(o, option.WithServiceAccountFile(keyfile))
	}
	return storage.NewClient(ctx, o...)
}

func isGsPath(path string) bool {
	return len(path) > 5 && path[:5] == "gs://"
}

func splitGsPath(path string) (string, string) {
	bits := strings.SplitN(path[5:], "/", 2)
	if len(bits) == 1 {
		return bits[0], ""
	}
	return bits[0], bits[1]
}

func printHelp(status int) {
	fmt.Println("usage: gslock <remote_file> <command>")
	os.Exit(status)
}

func init() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	flag.Parse()
}

func main() {
	args := flag.Args()
	if len(args) < 2 {
		printHelp(1)
	}

	path := args[0]
	if !isGsPath(path) {
		printHelp(1)
	}
	bucket, file := splitGsPath(path)
	if file == "" {
		printHelp(1)
	}

	os.Exit(run(bucket, file))
}

func run(bucket, file string) int {
	args := flag.Args()
	ctx := context.Background()
	client, err := newStorageClient(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	// Create our lock file if it doesn't exist
	// If it does exist, bail out.
	lockFile := client.Bucket(bucket).Object(file)
	gen := int64(0)
	for {
		w := lockFile.If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
		err := func() error {
			if _, err := w.Write([]byte("")); err != nil {
				return err
			}
			return w.Close()
		}()
		if err == nil {
			gen = w.Attrs().Generation
			break
		}
		if err := err.(*googleapi.Error); err.Code == 412 {
			time.Sleep(time.Second)
			continue
		}
		fmt.Println(os.Stderr, err.Error())
		return 1
	}

	// Make sure to release our lock when finished.
	defer func() {
		if err := lockFile.If(storage.Conditions{GenerationMatch: gen}).Delete(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}()

	// Execute our child process
	child := exec.Command(args[1], args[2:]...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Stdin = os.Stdin
	err = child.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	exitStatus := 0
	if !child.ProcessState.Success() {
		exitStatus = 1
	}
	// Try to fetch the actual status code if we can
	if status, ok := child.ProcessState.Sys().(syscall.WaitStatus); ok {
		exitStatus = status.ExitStatus()
	}

	return exitStatus
}
