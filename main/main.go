package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/coryb/optigo"
	"gopkg.in/Netflix-Skunkworks/go-jira.v0"
	"gopkg.in/coryb/yaml.v2"
	"gopkg.in/op/go-logging.v1"
)

var (
	log           = logging.MustGetLogger("jira")
	defaultFormat = "%{color}%{time:2006-01-02T15:04:05.000Z07:00} %{level:-5s} [%{shortfile}]%{color:reset} %{message}"
)

func main() {
	logBackend := logging.NewLogBackend(os.Stderr, "", 0)
	format := os.Getenv("JIRA_LOG_FORMAT")
	if format == "" {
		format = defaultFormat
	}
	logging.SetBackend(
		logging.NewBackendFormatter(
			logBackend,
			logging.MustStringFormatter(format),
		),
	)
	logging.SetLevel(logging.NOTICE, "")

	user := os.Getenv("USER")
	home := os.Getenv("HOME")
	defaultQueryFields := "summary,created,updated,priority,status,reporter,assignee"
	defaultSort := "priority asc, key"
	defaultMaxResults := 500

	usage := func(ok bool) {
		printer := fmt.Printf
		if !ok {
			printer = func(format string, args ...interface{}) (int, error) {
				return fmt.Fprintf(os.Stderr, format, args...)
			}
			defer func() {
				os.Exit(1)
			}()
		} else {
			defer func() {
				os.Exit(0)
			}()
		}
		output := fmt.Sprintf(`
Usage:
  jira (ls|list) <Query Options>
  jira view ISSUE
  jira worklog ISSUE
  jira add worklog ISSUE <Worklog Options>
  jira edit [--noedit] <Edit Options> [ISSUE | <Query Options>]
  jira create [--noedit] [-p PROJECT] <Create Options>
  jira subtask ISSUE [--noedit] <Create Options>
  jira DUPLICATE dups ISSUE
  jira BLOCKER blocks ISSUE
  jira issuelink OUTWARDISSUE ISSUELINKTYPE INWARDISSUE
  jira vote ISSUE [--down]
  jira rank ISSUE (after|before) ISSUE
  jira watch ISSUE [-w WATCHER] [--remove]
  jira (trans|transition) TRANSITION ISSUE [--noedit] <Edit Options>
  jira ack ISSUE [--edit] <Edit Options>
  jira close ISSUE [--edit] <Edit Options>
  jira resolve ISSUE [--edit] <Edit Options>
  jira reopen ISSUE [--edit] <Edit Options>
  jira start ISSUE [--edit] <Edit Options>
  jira stop ISSUE [--edit] <Edit Options>
  jira todo ISSUE [--edit] <Edit Options>
  jira backlog ISSUE [--edit] <Edit Options>
  jira done ISSUE [--edit] <Edit Options>
  jira prog|progress|in-progress [--edit] <Edit Options>
  jira comment ISSUE [--noedit] <Edit Options>
  jira (set,add,remove) labels ISSUE [LABEL] ...
  jira take ISSUE
  jira (assign|give) ISSUE [ASSIGNEE|--default]
  jira unassign ISSUE
  jira fields
  jira issuelinktypes
  jira transmeta ISSUE
  jira editmeta ISSUE
  jira add component [-p PROJECT] NAME DESCRIPTION LEAD
  jira components [-p PROJECT]
  jira issuetypes [-p PROJECT]
  jira createmeta [-p PROJECT] [-i ISSUETYPE]
  jira transitions ISSUE
  jira export-templates [-d DIR] [-t template]
  jira (b|browse) ISSUE
  jira (pr|pullrequest) PROJECT/REPO
  jira (repo|repository) PROJECT/REPO/[BRANCH]
  jira login
  jira logout
  jira request [-M METHOD] URI [DATA]
  jira ISSUE

General Options:
  -b --browse         Open your browser to the Jira issue
  -e --endpoint=URI   URI to use for jira
  -k --insecure       disable TLS certificate verification
  -h --help           Show this usage
  -t --template=FILE  Template file to use for output/editing
  -u --user=USER      Username to use for authenticaion (default: %s)
  -v --verbose        Increase output logging
  --unixproxy=PATH    Path for a unix-socket proxy (eg., --unixproxy /tmp/proxy.sock)
  --version           Print version

Query Options:
  -a --assignee=USER        Username assigned the issue
  -c --component=COMPONENT  Component to Search for
  -f --queryfields=FIELDS   Fields that are used in "list" template: (default: %s)
  -i --issuetype=ISSUETYPE  The Issue Type
  -l --limit=VAL            Maximum number of results to return in query (default: %d)
  --start=START             Start parameter for pagination
  -p --project=PROJECT      Project to Search for
  -q --query=JQL            Jira Query Language expression for the search
  -r --reporter=USER        Reporter to search for
  -s --sort=ORDER           For list operations, sort issues (default: %s)
  -w --watcher=USER         Watcher to add to issue (default: %s)
                            or Watcher to search for

Edit Options:
  -m --comment=COMMENT      Comment message for transition
  -o --override=KEY=VAL     Set custom key/value pairs

Create Options:
  -i --issuetype=ISSUETYPE  Jira Issue Type (default: Bug)
  -m --comment=COMMENT      Comment message for transition
  -o --override=KEY=VAL     Set custom key/value pairs

Worklog Options:
  -T --time-spent=TIMESPENT Time spent working on issue
  -m --comment=COMMENT      Comment message for worklog

Command Options:
  -d --directory=DIR        Directory to export templates to (default: %s)
`, user, defaultQueryFields, defaultMaxResults, defaultSort, user, fmt.Sprintf("%s/.jira.d/templates", home))
		printer(output)
	}

	jiraCommands := map[string]string{
		"list":             "list",
		"ls":               "list",
		"view":             "view",
		"edit":             "edit",
		"create":           "create",
		"subtask":          "subtask",
		"dups":             "dups",
		"blocks":           "blocks",
		"issuelink":        "issuelink",
		"watch":            "watch",
		"trans":            "transition",
		"transition":       "transition",
		"ack":              "acknowledge",
		"acknowledge":      "acknowledge",
		"close":            "close",
		"resolve":          "resolve",
		"reopen":           "reopen",
		"start":            "start",
		"stop":             "stop",
		"todo":             "todo",
		"backlog":          "backlog",
		"done":             "done",
		"prog":             "in-progress",
		"progress":         "in-progress",
		"in-progress":      "in-progress",
		"comment":          "comment",
		"label":            "labels",
		"labels":           "labels",
		"component":        "component",
		"components":       "components",
		"take":             "take",
		"assign":           "assign",
		"give":             "assign",
		"fields":           "fields",
		"issuelinktypes":   "issuelinktypes",
		"transmeta":        "transmeta",
		"editmeta":         "editmeta",
		"issuetypes":       "issuetypes",
		"createmeta":       "createmeta",
		"transitions":      "transitions",
		"export-templates": "export-templates",
		"browse":           "browse",
		"pullrequest":      "pullrequest",
		"pr":               "pullrequest",
		"repo":             "repository",
		"repository":       "repository",
		"login":            "login",
		"logout":           "logout",
		"req":              "request",
		"request":          "request",
		"vote":             "vote",
		"rank":             "rank",
		"worklog":          "worklog",
		"addworklog":       "addworklog",
		"unassign":         "unassign",
	}

	defaults := map[string]interface{}{
		"user":        user,
		"queryfields": defaultQueryFields,
		"directory":   fmt.Sprintf("%s/.jira.d/templates", home),
		"sort":        defaultSort,
		"max_results": defaultMaxResults,
		"method":      "GET",
		"quiet":       false,
	}
	opts := make(map[string]interface{})

	setopt := func(name string, value interface{}) {
		opts[name] = value
	}

	op := optigo.NewDirectAssignParser(map[string]interface{}{
		"h|help": usage,
		"version": func() {
			fmt.Println(fmt.Sprintf("version: %s", jira.VERSION))
			os.Exit(0)
		},
		"v|verbose+": func() {
			logging.SetLevel(logging.GetLevel("")+1, "")
		},
		"dryrun":                setopt,
		"b|browse":              setopt,
		"editor=s":              setopt,
		"u|user=s":              setopt,
		"endpoint=s":            setopt,
		"stashEndpoint=s":       setopt,
		"k|insecure":            setopt,
		"t|template=s":          setopt,
		"q|query=s":             setopt,
		"p|project=s":           setopt,
		"c|component=s":         setopt,
		"a|assignee=s":          setopt,
		"i|issuetype=s":         setopt,
		"w|watcher=s":           setopt,
		"remove":                setopt,
		"r|reporter=s":          setopt,
		"f|queryfields=s":       setopt,
		"x|expand=s":            setopt,
		"s|sort=s":              setopt,
		"l|limit|max_results=i": setopt,
		"start|start_at=i":      setopt,
		"o|override=s%":         &opts,
		"noedit":                setopt,
		"edit":                  setopt,
		"m|comment=s":           setopt,
		"d|dir|directory=s":     setopt,
		"M|method=s":            setopt,
		"S|saveFile=s":          setopt,
		"T|time-spent=s":        setopt,
		"Q|quiet":               setopt,
		"unixproxy":             setopt,
		"down":                  setopt,
		"default":               setopt,
	})

	if err := op.ProcessAll(os.Args[1:]); err != nil {
		log.Errorf("%s", err)
		usage(false)
	}
	args := op.Args

	var command string
	if len(args) > 0 {
		if alias, ok := jiraCommands[args[0]]; ok {
			command = alias
			args = args[1:]
		} else if len(args) > 1 {
			// look at second arg for "dups" and "blocks" commands
			// also for 'set/add/remove' actions like 'labels'
			if alias, ok := jiraCommands[args[1]]; ok {
				command = alias
				args = append(args[:1], args[2:]...)
			}
		}
	}

	if command == "" && len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	os.Setenv("JIRA_OPERATION", command)
	loadConfigs(opts)

	// check to see if it was set in the configs:
	if value, ok := opts["command"].(string); ok {
		command = value
	} else if _, ok := jiraCommands[command]; !ok || command == "" {
		if command != "" {
			args = append([]string{command}, args...)
		}
		command = "view"
	}

	// apply defaults
	for k, v := range defaults {
		if _, ok := opts[k]; !ok {
			log.Debugf("Setting %q to %#v from defaults", k, v)
			opts[k] = v
		}
	}

	log.Debugf("opts: %v", opts)
	log.Debugf("args: %v", args)

	if _, ok := opts["endpoint"]; !ok {
		log.Errorf("endpoint option required.  Either use --endpoint or set a endpoint option in your ~/.jira.d/config.yml file")
		os.Exit(1)
	}

	c := jira.New(opts)


	log.Debugf("opts: %s", opts)

	setEditing := func(dflt bool) {
		log.Debugf("Default Editing: %t", dflt)
		if dflt {
			if val, ok := opts["noedit"].(bool); ok && val {
				log.Debugf("Setting edit = false")
				opts["edit"] = false
			} else {
				log.Debugf("Setting edit = true")
				opts["edit"] = true
			}
		} else {
			if _, ok := opts["edit"].(bool); !ok {
				log.Debugf("Setting edit = %t", dflt)
				opts["edit"] = dflt
			}
		}
	}

	requireArgs := func(count int) {
		if len(args) < count {
			log.Errorf("Not enough arguments. %d required, %d provided", count, len(args))
			usage(false)
		}
	}

	var err error
	switch command {
	case "issuelink":
		requireArgs(3)
		err = c.CmdIssueLink(args[0], args[1], args[2])
	case "login":
		err = c.CmdLogin()
	case "logout":
		err = c.CmdLogout()
	case "fields":
		err = c.CmdFields()
	case "list":
		err = c.CmdList()
	case "edit":
		setEditing(true)
		if len(args) > 0 {
			err = c.CmdEdit(args[0])
		} else {
			var data interface{}
			if data, err = c.FindIssues(); err == nil {
				issues := data.(map[string]interface{})["issues"].([]interface{})
				for _, issue := range issues {
					if err = c.CmdEdit(issue.(map[string]interface{})["key"].(string)); err != nil {
						switch err.(type) {
						case jira.NoChangesFound:
							log.Warning("No Changes found: %s", err)
							err = nil
							continue
						}
						break
					}
				}
			}
		}
	case "editmeta":
		requireArgs(1)
		err = c.CmdEditMeta(args[0])
	case "transmeta":
		requireArgs(1)
		err = c.CmdTransitionMeta(args[0])
	case "issuelinktypes":
		err = c.CmdIssueLinkTypes()
	case "issuetypes":
		err = c.CmdIssueTypes()
	case "createmeta":
		err = c.CmdCreateMeta()
	case "create":
		setEditing(true)
		err = c.CmdCreate()
	case "subtask":
		setEditing(true)
		err = c.CmdSubtask(args[0])
	case "transitions":
		requireArgs(1)
		err = c.CmdTransitions(args[0])
	case "blocks":
		requireArgs(2)
		err = c.CmdBlocks(args[0], args[1])
	case "dups":
		setEditing(true)
		requireArgs(2)
		if err = c.CmdDups(args[0], args[1]); err == nil {
			opts["resolution"] = "Duplicate"
			trans, err := c.ValidTransitions(args[0])
			if err == nil {
				if trans.Find("close") != nil {
					err = c.CmdTransition(args[0], "close")
				} else if trans.Find("done") != nil {
					// for now just assume if there is no "close", then
					// there is a "done" state
					err = c.CmdTransition(args[0], "done")
				} else if trans.Find("start") != nil {
					err = c.CmdTransition(args[0], "start")
					if err == nil {
						err = c.CmdTransition(args[0], "stop")
					}
				}
			}

		}
	case "watch":
		requireArgs(1)
		watcher := c.GetOptString("watcher", opts["user"].(string))
		remove := c.GetOptBool("remove", false)
		err = c.CmdWatch(args[0], watcher, remove)
	case "transition":
		requireArgs(2)
		setEditing(true)
		err = c.CmdTransition(args[1], args[0])
	case "close":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "close")
	case "acknowledge":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "acknowledge")
	case "reopen":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "reopen")
	case "resolve":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "resolve")
	case "start":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "start")
	case "stop":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "stop")
	case "todo":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "To Do")
	case "backlog":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "Backlog")
	case "done":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "Done")
	case "in-progress":
		requireArgs(1)
		setEditing(false)
		err = c.CmdTransition(args[0], "Progress")
	case "comment":
		requireArgs(1)
		setEditing(true)
		err = c.CmdComment(args[0])
	case "labels":
		requireArgs(2)
		action := args[0]
		issue := args[1]
		labels := args[2:]
		err = c.CmdLabels(action, issue, labels)
	case "component":
		requireArgs(2)
		action := args[0]
		project := opts["project"].(string)
		name := args[1]
		var lead string
		var description string
		if len(args) > 2 {
			description = args[2]
		}
		if len(args) > 3 {
			lead = args[2]
		}
		err = c.CmdComponent(action, project, name, description, lead)
	case "components":
		project := opts["project"].(string)
		err = c.CmdComponents(project)
	case "take":
		requireArgs(1)
		err = c.CmdAssign(args[0], opts["user"].(string))
	case "browse":
		requireArgs(1)
		opts["browse"] = true
		err = c.Browse(args[0])
	case "pullrequest":
		requireArgs(1)
		opts["pullrequest"] = true
		err = c.BrowsePullRequest(args[0])
	case "repository":
		requireArgs(1)
		opts["repository"] = true

		err = c.BrowseRepository(args[0])
	case "export-templates":
		err = c.CmdExportTemplates()
	case "assign":
		requireArgs(1)
		assignee := ""
		if len(args) > 1 {
			assignee = args[1]
		}
		err = c.CmdAssign(args[0], assignee)
	case "unassign":
		requireArgs(1)
		err = c.CmdUnassign(args[0])
	case "view":
		requireArgs(1)
		err = c.CmdView(args[0])
	case "worklog":
		if len(args) > 0 && args[0] == "add" {
			setEditing(true)
			requireArgs(2)
			err = c.CmdWorklog(args[0], args[1])
		} else {
			requireArgs(1)
			err = c.CmdWorklogs(args[0])
		}
	case "vote":
		requireArgs(1)
		if val, ok := opts["down"]; ok {
			err = c.CmdVote(args[0], !val.(bool))
		} else {
			err = c.CmdVote(args[0], true)
		}
	case "rank":
		requireArgs(3)
		if args[1] == "after" {
			err = c.CmdRankAfter(args[0], args[2])
		} else {
			err = c.CmdRankBefore(args[0], args[2])
		}
	case "request":
		requireArgs(1)
		data := ""
		if len(args) > 1 {
			data = args[1]
		}
		err = c.CmdRequest(args[0], data)
	default:
		log.Errorf("Unknown command %s", command)
		os.Exit(1)
	}

	if err != nil {
		log.Errorf("%s", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func parseYaml(file string, opts map[string]interface{}) {
	if fh, err := ioutil.ReadFile(file); err == nil {
		log.Debugf("Found Config file: %s", file)
		if err := yaml.Unmarshal(fh, &opts); err != nil {
			log.Errorf("Unable to parse %s: %s", file, err)
		}
	}
}

func populateEnv(opts map[string]interface{}) {
	for k, v := range opts {
		envName := fmt.Sprintf("JIRA_%s", strings.ToUpper(k))
		var val string
		switch t := v.(type) {
		case string:
			val = t
		case int, int8, int16, int32, int64:
			val = fmt.Sprintf("%d", t)
		case float32, float64:
			val = fmt.Sprintf("%f", t)
		case bool:
			val = fmt.Sprintf("%t", t)
		default:
			val = fmt.Sprintf("%v", t)
		}
		os.Setenv(envName, val)
	}
}

func loadConfigs(opts map[string]interface{}) {
	populateEnv(opts)
	paths := jira.FindParentPaths(".jira.d/config.yml")
	// prepend
	paths = append(paths, "/etc/go-jira.yml")

	// iterate paths in reverse
	for i := 0; i < len(paths); i++ {
		file := paths[i]
		if stat, err := os.Stat(file); err == nil {
			tmp := make(map[string]interface{})
			// check to see if config file is exectuable
			if stat.Mode()&0111 == 0 {
				parseYaml(file, tmp)
			} else {
				log.Debugf("Found Executable Config file: %s", file)
				// it is executable, so run it and try to parse the output
				cmd := exec.Command(file)
				stdout := bytes.NewBufferString("")
				cmd.Stdout = stdout
				cmd.Stderr = bytes.NewBufferString("")
				if err := cmd.Run(); err != nil {
					log.Errorf("%s is exectuable, but it failed to execute: %s\n%s", file, err, cmd.Stderr)
					os.Exit(1)
				}
				yaml.Unmarshal(stdout.Bytes(), &tmp)
			}
			for k, v := range tmp {
				if _, ok := opts[k]; !ok {
					log.Debugf("Setting %q to %#v from %s", k, v, file)
					opts[k] = v
				}
			}
			populateEnv(opts)
		}
	}
}
