package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cheggaaa/pb"
	"github.com/howeyc/gopass"
	"github.com/mxk/go-flowrate/flowrate"
	"golang.org/x/crypto/ssh"
)

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
	limit := flag.Int64("limit", 0, "bandwidth limit in bytes/sec")
	verbose := flag.Bool("v", false, "show verbose messages")
	fileListing := flag.Bool("ls", false, "folder listing")
	flag.Parse()

	if flag.NArg() < 2 && !*fileListing {
		fmt.Println("Must specify at least one source and a destination.")
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	targetUser, targetHost, targetFile := parseFileHostLocation(args[len(args)-1])

	password := func() (secret string, err error) {
		if *pw != "" {
			return *pw, nil
		}
		fmt.Print("Password: ")
		pass := string(gopass.GetPasswd())
		return pass, nil
	}

	var targetClient *ssh.Client
	if targetHost != "" {
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

		var clientErr error
		targetClient, clientErr = connectToRemoteHost(ssh.PasswordCallback(password), *user, targetHost, *port)
		if clientErr != nil {
			log.Fatalln("Failed to dial: " + clientErr.Error())
		}
	}

	// Handle -ls flag
	if *fileListing {
		displayListing(targetClient, targetFile)
		os.Exit(0)
	}

	for _, sourceFile := range args[:len(args)-1] {
		srcUser, srcHost, srcFile := parseFileHostLocation(sourceFile)
		if srcUser != "" && srcHost != "" {
			client, err := connectToRemoteHost(ssh.PasswordCallback(password), srcUser, srcHost, 22)
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

// Support keyboard interactive challenge
func kic(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
	if user != "" || instruction != "" {
		fmt.Println(user, instruction)
	}
	for idx, question := range questions {
		fmt.Print(question)
		if !echos[idx] {
			answers = append(answers, string(gopass.GetPasswd()))
		} else {
			bufin := bufio.NewReader(os.Stdin)
			line, _ := bufin.ReadString('\n')
			answers = append(answers, line)
		}
	}
	return answers, nil
}

func connectToRemoteHost(auth ssh.AuthMethod, user, host string, port int64) (*ssh.Client, error) {
	clientConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{auth, ssh.KeyboardInteractive(kic)},
	}

	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), clientConfig)
}

func displayListing(client *ssh.Client, targetFile string) {
	session, err := client.NewSession()
	if err != nil {
		log.Fatalln("Failed to create session: " + err.Error())
	}
	defer session.Close()

	go func() {
		or, err := session.StdoutPipe()
		if err != nil {
			log.Fatalln("Failed to create output pipe: " + err.Error())
		}
		io.Copy(os.Stdout, or)
	}()

	if err := session.Run(fmt.Sprintf("ls -al %s", targetFile)); err != nil {
		log.Fatalln("Failed to run: " + err.Error())
	}
}

func sendFileToRemoteHost(client *ssh.Client, limit int64, sourceFile, targetUser, targetHost, targetFile string) {
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

		w := flowrate.NewWriter(iw, limit)
		src, srcErr := os.Open(sourceFile)
		if srcErr != nil {
			log.Fatalln("Failed to open source file: " + srcErr.Error())
		}
		srcStat, statErr := src.Stat()
		if statErr != nil {
			log.Fatalln("Failed to stat file: " + statErr.Error())
		}

		fmt.Fprintln(w, "C0644", srcStat.Size(), filepath.Base(sourceFile))
		if srcStat.Size() > 0 {
			bar := pb.New(int(srcStat.Size()))
			bar.Units = pb.U_BYTES
			bar.ShowSpeed = true
			bar.Start()
			wp := io.MultiWriter(w, bar)

			fmt.Printf("Transferring %s to %s@%s:%s\n", sourceFile, targetUser, targetHost, targetFile)
			fmt.Printf("Speed limited to %d bytes/sec\n", limit)

			io.Copy(wp, src)
			bar.Finish()
			fmt.Fprint(w, "\x00")
			w.Close()
		} else {
			fmt.Printf("Transferred empty file %s to %s@%s:%s\n", sourceFile, targetUser, targetHost, targetFile)
			fmt.Fprint(w, "\x00")
			w.Close()
		}
	}()
	if err := session.Run(fmt.Sprintf("scp -t %s", targetFile)); err != nil {
		log.Fatalln("Failed to run: " + err.Error())
	}
}

func getFileFromRemoteHost(client *ssh.Client, localFile, targetUser, targetHost, targetFile string) {
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
			log.Fatalln("Failed to create output pipe: " + err.Error())
		}
		fmt.Fprint(iw, "\x00")

		sr := bufio.NewReader(or)

		src, srcErr := os.Create(localFile)
		if srcErr != nil {
			log.Fatalln("Failed to create source file: " + srcErr.Error())
		}
		if controlString, ok := sr.ReadString('\n'); ok == nil && strings.HasPrefix(controlString, "C") {
			fmt.Fprint(iw, "\x00")
			controlParts := strings.Split(controlString, " ")
			size, _ := strconv.ParseInt(controlParts[1], 10, 64)
			bar := pb.New(int(size))
			bar.Units = pb.U_BYTES
			bar.ShowSpeed = true
			bar.Start()
			rp := io.MultiReader(sr, bar)
			if n, ok := io.CopyN(src, rp, size); ok != nil || n < size {
				fmt.Println(n)
				fmt.Fprint(iw, "\x02")
				return
			}
			bar.Finish()
			sr.Read(make([]byte, 1))
		}
		fmt.Fprint(iw, "\x00")
	}()
	if err := session.Run(fmt.Sprintf("scp -f %s", targetFile)); err != nil {
		log.Fatalln("Failed to run: " + err.Error())
	}
}
