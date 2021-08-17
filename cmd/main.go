package main

import (
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type MailMessage struct {
	Otp string    `json:"otp"`
	To  string    `json:"to"`
	Exp time.Time `json:"exp"`
}

const (
	errSubj = "NUBES3.nubes3_err"
)

var (
	serviceEmail string
	sgKey        string
	//sc stan.Conn
	nc *nats.Conn
	js nats.JetStreamContext
)

func main() {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	sgKey = viper.GetString("SG_api_key")
	if sgKey == "" {
		panic("Fatal error Missing SG API Key\n")
	}
	serviceEmail = viper.GetString("Sg_email")
	if serviceEmail == "" {
		panic("Fatal error Missing SG Email\n")
	}

	qsub := getMailQSub()

	sigs := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = qsub.Unsubscribe()
		cleanupDone <- true
	}()

	fmt.Println("Listening for mail events")
	<-cleanupDone
}

func sendEmail(msg MailMessage) error {
	from := mail.NewEmail("Nubes3", serviceEmail)
	subject := "Verify Email"
	to := mail.NewEmail(msg.To, msg.To)
	content :=
		"Here is your otp code:\r\n\r\n" + msg.Otp + "\r\n\r\n" +
			"The OTP will be expired at " + msg.Exp.Local().Format(time.RFC3339) + ". Please do not share it to the public.\r\n\r\nRegards."
	message := mail.NewSingleEmail(from, subject, to, content, "")
	client := sendgrid.NewSendClient(sgKey)
	_, err := client.Send(message)
	return err
}

func getMailQSub() *nats.Subscription {
	var err error
	//sc, err = stan.Connect(viper.GetString("Cluster_id"), viper.GetString("Client_id") + stringutil.RandomStrings(5, 1)[0], stan.NatsURL("nats://"+viper.GetString("Nats_url")))
	//if err != nil {
	//	panic(fmt.Errorf("Fatal error connecting nats stream: %s \n", err))
	//}
	nc, err = nats.Connect("nats://" + viper.GetString("Nats_url"))
	if err != nil {
		panic(err)
	}

	js, err = nc.JetStream()
	if err != nil {
		panic(err)
	}

	qsub, _ := js.QueueSubscribe(viper.GetString("Mail_subject"), "mail-qgroup", func(msg *nats.Msg) {
		go func() {
			var data MailMessage
			_ = json.Unmarshal(msg.Data, &data)
			err = sendEmail(data)
			if err != nil {
				err = SendErrorEvent(err.Error(), "mail")
				if err != nil {
					println(err)
				}
			}
		}()
		msg.Ack()
	})

	return qsub
}

type Event struct {
	Type string    `json:"type"`
	Date time.Time `json:"at"`
}

type ErrLogMessage struct {
	Event
	Content string `json:"content"`
}

func SendErrorEvent(content, t string) error {
	jsonData, err := json.Marshal(ErrLogMessage{
		Event: Event{
			Type: t,
			Date: time.Now(),
		},
		Content: content,
	})

	if err != nil {
		return err
	}

	//return sc.Publish(errSubj, jsonData)
	_, err = js.Publish(errSubj, jsonData)
	return err
}
