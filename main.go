package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Flicster/peerchat/internal/app/service"
	"github.com/sirupsen/logrus"
)

const figlet = `

W E L C O M E  T O
					     db                  db   
					     88                  88   
.8d888b. .d8888b. .d8888b. .d8888b. .d8888b. 88d888b. .d8888b. d8888P 
88'  '88 88ooood8 88ooood8 88'  '88 88'      88'  '88 88'  '88   88   
88.  .88 88.      88.      88       88.      88    88 88.  .88   88   
888888P' '88888P' '88888P' db       '88888P' db    db '8888888   '88P   
88                                                                    
dP`

func init() {
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	})

	logrus.SetOutput(os.Stdout)
}

func main() {
	username := flag.String("user", "", "username to use in the chatroom.")
	chatroom := flag.String("room", "", "chatroom to join.")
	loglevel := flag.String("log", "", "level of logs to print.")

	flag.Parse()

	switch *loglevel {
	case "panic", "PANIC":
		logrus.SetLevel(logrus.PanicLevel)
	case "fatal", "FATAL":
		logrus.SetLevel(logrus.FatalLevel)
	case "error", "ERROR":
		logrus.SetLevel(logrus.ErrorLevel)
	case "warn", "WARN":
		logrus.SetLevel(logrus.WarnLevel)
	case "info", "INFO":
		logrus.SetLevel(logrus.InfoLevel)
	case "debug", "DEBUG":
		logrus.SetLevel(logrus.DebugLevel)
	case "trace", "TRACE":
		logrus.SetLevel(logrus.TraceLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	fmt.Println(figlet)
	fmt.Println()
	fmt.Println("The PeerChat Application is starting.")
	fmt.Println("This may take upto 30 seconds.")
	fmt.Println()

	p2p, err := service.NewP2P()
	if err != nil {
		logrus.Fatal(err)
	}

	fmt.Println("Completed P2P Setup.")
	fmt.Println("Connecting to Service Peers...")

	err = p2p.AdvertiseConnect()
	if err != nil {
		logrus.Fatal(err)
	}

	chat, err := service.NewChatRoom(p2p, *username, *chatroom)
	if err != nil {
		logrus.Fatal(err)
	}

	ui := service.NewUI(chat)
	if err = ui.Run(); err != nil {
		logrus.Fatal(err)
	}
}
