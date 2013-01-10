package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.google.com/p/go.crypto/ssh"
	"code.google.com/p/mxk/go1/flowcontrol"
	"github.com/cheggaaa/pb"
	"github.com/howeyc/gopass"
)

type cred struct {
	user, pass string
}

func (c *cred) Password(user string) (password string, err error) {
	if c.pass == "" {
		fmt.Printf("Password: ")
		c.pass = string(gopass.GetPasswd())
	}
	if user == c.user {
		return c.pass, nil
	}
	return "", errors.New("Invalid User.")
}

func parseFileHostLocation(loc string) (user, host, path string) {
	path = loc
	sp1 := strings.Split(loc, "@")
	if len(sp1) == 2 {
		user = sp1[0]
		sp1 = strings.Split(sp1[1], ":")
	} else {
		sp1 = strings.Split(sp1[0], ":")
	}
	if len(sp1) == 2 {
		host = sp1[0]
		path = sp1[1]
	}
	return
}

func main() {
	user := flag.String("l", "", "connect with specified username")
	pw := flag.String("pw", "", "login with specified password")
	port := flag.Int("P", 22, "connect with specified port")
	limit := flag.Int64("limit", 1024, "bandwidth limit in bytes/sec")
	flag.Parse()

	if flag.NArg() < 2 {
		fmt.Println("Must specify at least one source and a destination.")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	targetUser, targetHost, targetFile := parseFileHostLocation(args[len(args)-1])

	if targetUser != "" && *user != "" && targetUser != *user {
		fmt.Println("Specfied user@host and -l user that do not match.")
		flag.Usage()
		os.Exit(1)
	}

	if targetUser == "" && *user == "" {
		fmt.Println("Must specify username.")
		flag.Usage()
		os.Exit(1)
	}

	if *user == "" {
		*user = targetUser
	}

	clientCred := &cred{*user, *pw}
	clientConfig := &ssh.ClientConfig{
		User: *user,
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthPassword(clientCred),
		},
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", targetHost, *port), clientConfig)
	if err != nil {
		log.Fatalln("Failed to dial: " + err.Error())
	}

	for _, sourceFile := range args[:len(args)-1] {
		sendFileToRemoteHost(client, *limit, sourceFile, targetUser, targetHost, targetFile)
	}
}

func sendFileToRemoteHost(client *ssh.ClientConn, limit int64, sourceFile, targetUser, targetHost, targetFile string) {
	session, err := client.NewSession()
	if err != nil {
		log.Fatalln("Failed to create session: " + err.Error())
	}
	defer session.Close()

	go func() {
		iw, err := session.StdinPipe()
		if err != nil {
			log.Fatalln("Failed to create input pipe: " + err.Error())
		}

		w := flowcontrol.NewWriter(iw, limit)
		src, srcErr := os.Open(sourceFile)
		if srcErr != nil {
			log.Fatalln("Failed to open source file: " + srcErr.Error())
		}
		srcStat, statErr := src.Stat()
		if statErr != nil {
			log.Fatalln("Failed to stat file: " + statErr.Error())
		}
		fmt.Fprintln(w, "C0644", srcStat.Size(), filepath.Base(sourceFile))
		wp := &writeProgress{w, pb.StartNew(int(srcStat.Size())), time.Now()}

		fmt.Printf("Transferring %s to %s@%s:%s\n", sourceFile, targetUser, targetHost, targetFile)
		fmt.Printf("Speed limited to %d bytes/sec\n", limit)

		io.Copy(wp, src)
		fmt.Fprint(w, "\x00")
		wp.Close()
	}()
	if err := session.Run(fmt.Sprintf("scp -t %s", targetFile)); err != nil {
		log.Fatalln("Failed to run: " + err.Error())
	}
}
