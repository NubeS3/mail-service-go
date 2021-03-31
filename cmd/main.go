package main

import (
	"encoding/json"
	"fmt"
	stan "github.com/nats-io/stan.go"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type MailMessage struct {
	Otp      string    `json:"otp"`
	Username string    `json:"username"`
	To       string    `json:"to"`
	Exp      time.Time `json:"exp"`
}

var (
	serviceEmail string
	sgKey        string
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

	qsub := getMailQSub(err)

	sigs := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = qsub.Unsubscribe()
		_ = qsub.Close()
		cleanupDone <- true
	}()

	<-cleanupDone
}

func sendEmail(msg MailMessage) error {
	from := mail.NewEmail("Nubes3", serviceEmail)
	subject := "Verify Email"
	to := mail.NewEmail(msg.Username, msg.To)
	content :=
		"Enter the OTP we sent you via email to continue.\r\n\r\n" + msg.Otp + "\r\n\r\n" +
			"The OTP will be expired at " + msg.Exp.Local().Format("02-01-2006 15:04") + ". Do not share it to public."
	message := mail.NewSingleEmail(from, subject, to, content, "")
	client := sendgrid.NewSendClient(sgKey)
	_, err := client.Send(message)
	return err
}

func getMailQSub(err error) stan.Subscription {
	sc, err := stan.Connect(viper.GetString("Cluster_id"), viper.GetString("Client_id"), stan.NatsURL(viper.GetString("Nats_url")))
	if err != nil {
		panic(fmt.Errorf("Fatal error connecting nats stream: %s \n", err))
	}

	qsub, _ := sc.QueueSubscribe(viper.GetString("Mail_subject"), "mail-qgroup", func(msg *stan.Msg) {
		go func() {
			var data MailMessage
			_ = json.Unmarshal(msg.Data, &data)
			_ = sendEmail(data)
			//TODO log email error
		}()
	})
	return qsub
}
