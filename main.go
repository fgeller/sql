package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type database struct {
	AppServer string
	DbServer  string
	DbName    string
	User      string
	Pass      string
}

func main() {
	databases := mustReadDatabasesConfigFile()

	if len(os.Args[1:]) == 0 {
		usage("Target database unspecified; where should I run the query?")
	}

	sql := readInput(os.Stdin)
	if len(sql) <= 3 {
		usage("No SQL to run. Exiting.")
	}

	targetDatabases := []string{}
	for _, k := range os.Args[1:] {
		if _, ok := databases[k]; k != "all" && !ok {
			usage("Target database unknown: [%v]", k)
		}
		if k == "all" {
			targetDatabases = nil
			for k := range databases {
				targetDatabases = append(targetDatabases, k)
			}
			break
		}
		targetDatabases = append(targetDatabases, k)
	}

	var wg sync.WaitGroup

	finalResult := true
	for _, k := range targetDatabases {
		wg.Add(1)
		go func(db database, k string) {
			defer wg.Done()
			if r := runSQL(db, sql, k, len(targetDatabases) > 1); !r {
				finalResult = false
			}
		}(databases[k], k)
	}

	wg.Wait()

	if !finalResult {
		os.Exit(1)
	}
	os.Exit(0)
}

func runSQL(db database, sql string, key string, prependKey bool) bool {
	userOption := ""
	if db.User != "" {
		userOption = fmt.Sprintf("-u %v ", db.User)
	}

	passOption := ""
	if db.Pass != "" {
		passOption = fmt.Sprintf("-p%v ", db.Pass)
	}

	hostOption := ""
	if db.DbServer != "" {
		hostOption = fmt.Sprintf("-h %v ", db.DbServer)
	}

	prepend := ""
	if prependKey {
		prepend = key + "\t"
	}

	mysql := "mysql"
	options := fmt.Sprintf(" -Nsr %v%v%v%v -e ", userOption, passOption, hostOption, db.DbName)

	var cmd *exec.Cmd
	if db.AppServer != "" {
		query := fmt.Sprintf(`'%v'`, strings.Replace(sql, `'`, `'"'"'`, -1))
		cmd = exec.Command("ssh", db.AppServer, mysql+options+query)
	} else {
		args := append(trimEmpty(strings.Split(options, " ")), sql)
		cmd = exec.Command("mysql", args...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Cannot create pipe for STDOUT of running command on %v; not running.\n", key)
		return false
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Cannot create pipe for STDERR of running command on %v; not running.\n", key)
		return false
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Cannot start command on %v; not running.\n", key)
		return false
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		fmt.Println(prepend + scanner.Text())
	}

	stderrLines := []string{}
	scanner = bufio.NewScanner(stderr)
	for scanner.Scan() {
		stderrLines = append(stderrLines, scanner.Text())
	}

	cmd.Wait()

	result := true
	if len(stderrLines) > 0 {
		result = false
		log.Println(key + " had errors:")
		for _, v := range stderrLines {
			log.Println(key + " [ERROR] " + v)
		}
	}

	return result
}

func readInput(r io.Reader) string {
	ls := []string{}
	var err error
	rd := bufio.NewReader(r)

	for {
		var s string
		s, err = rd.ReadString('\n')
		if err == io.EOF {
			return strings.Join(ls, " ")
		}
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}
		ls = append(ls, strings.TrimSpace(s))
	}
}

func trimEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
