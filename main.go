package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/go.crypto/ssh"
	"code.google.com/p/mxk/go1/flowcontrol"
	"github.com/cheggaaa/pb"
	"github.com/howeyc/gopass"
)

type cred struct {
	user, host, pass string
}

func (c *cred) Password(user string) (password string, err error) {
	if user == c.user && c.pass == "" {
		fmt.Printf("Password for %s@%s: ", c.user, c.host)
		c.pass = string(gopass.GetPasswd())
		return c.pass, nil
	} else if user == c.user {
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
	port := flag.Int64("P", 22, "connect with specified port")
	limit := flag.Int64("limit", 10240, "bandwidth limit in bytes/sec")
	verbose := flag.Bool("v", false, "show verbose messages")
	flag.Parse()

	if flag.NArg() < 2 {
		fmt.Println("Must specify at least one source and a destination.")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	targetUser, targetHost, targetFile := parseFileHostLocation(args[len(args)-1])

	var targetClient *ssh.ClientConn
	if targetHost != "" {
		fmt.Println("Target Host: ", targetHost)
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

		clientCred := &cred{*user, targetHost, *pw}
		var clientErr error
		targetClient, clientErr = connectToRemoteHost(ssh.ClientAuthPassword(clientCred), *user, targetHost, *port)
		if clientErr != nil {
			log.Fatalln("Failed to dial: " + clientErr.Error())
		}

	}

	for _, sourceFile := range args[:len(args)-1] {
		srcUser, srcHost, srcFile := parseFileHostLocation(sourceFile)
		if srcUser != "" && srcHost != "" {
			clientCred := &cred{srcUser, srcHost, ""}
			client, err := connectToRemoteHost(ssh.ClientAuthPassword(clientCred), srcUser, srcHost, 22)
			if err != nil {
				log.Fatalln("Failed to dial: " + err.Error())
			}
			if targetHost == "" {
				if targetInfo, statErr := os.Stat(targetFile); statErr == nil && targetInfo.IsDir() == true {
					getFileFromRemoteHost(client, filepath.Join(targetFile, srcFile), srcUser, srcHost, srcFile)
				} else {
					getFileFromRemoteHost(client, targetFile, srcUser, srcHost, srcFile)
				}
			} else {
				fmt.Println("Both source and destination cannot be remote, one side must be local.")
				os.Exit(1)
			}
		} else {
			sendFileToRemoteHost(targetClient, *limit, sourceFile, targetUser, targetHost, targetFile)
		}
	}
	//PSCP has -v flag, so we need to use it as well
	if *verbose == true {
		fmt.Println("Program completed.")
	}
}

func connectToRemoteHost(auth ssh.ClientAuth, user, host string, port int64) (*ssh.ClientConn, error) {
	clientConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.ClientAuth{auth},
	}

	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), clientConfig)
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

func getFileFromRemoteHost(client *ssh.ClientConn, localFile, targetUser, targetHost, targetFile string) {
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
		or, err := session.StdoutPipe()
		if err != nil {
			log.Fatalln("Failed to create input pipe: " + err.Error())
		}
		fmt.Fprint(iw, "\x00")

		sr := bufio.NewReader(or)

		src, srcErr := os.Create(localFile)
		if srcErr != nil {
			log.Fatalln("Failed to create source file: " + srcErr.Error())
		}
		if controlString, ok := sr.ReadString('\n'); ok == nil && strings.HasPrefix(controlString, "C") {
			fmt.Fprint(iw, "\x00")
			fmt.Println(controlString)
			controlParts := strings.Split(controlString, " ")
			size, _ := strconv.ParseInt(controlParts[1], 10, 64)
			buf := make([]byte, size)
			if n, ok := io.ReadFull(sr, buf); ok != nil || n < len(buf) {
				fmt.Println(n)
				fmt.Fprint(iw, "\x02")
				return
			}
			src.Write(buf)
			sr.Read(buf[:1])
		}
		fmt.Fprint(iw, "\x00")
	}()
	if err := session.Run(fmt.Sprintf("scp -f %s", targetFile)); err != nil {
		log.Fatalln("Failed to run: " + err.Error())
	}
}
