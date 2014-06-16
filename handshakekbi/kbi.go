package handshakekbi

import (
	"log"

	"code.google.com/p/go.crypto/ssh"
)

type keyboardInteractive struct {
	user, instruction string
	questions         []string
	echos             []bool
	reply             chan []string
}

type HandshakeKBI struct{}

func (k *HandshakeKBI) Handshake(downstreamConf *ssh.ServerConfig, target string) <-chan *ssh.Client {
	authKBI := make(chan keyboardInteractive, 10)
	userChan := make(chan string, 10)
	upstreamConnected := make(chan error, 10)
	ua := ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
		log.Printf("upstream auth: %q %q %v", user, instruction, questions)
		q := keyboardInteractive{
			user:        user,
			instruction: instruction,
			questions:   questions,
			echos:       echos,
			reply:       make(chan []string, 10),
		}
		authKBI <- q
		ans := <-q.reply
		log.Printf("answering upstream")
		return ans, nil
	})

	upstreamConf := &ssh.ClientConfig{
		Auth: []ssh.AuthMethod{
			ua,
		},
	}

	downstreamConf.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, chal ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		userChan <- c.User()
		for try := range authKBI {
			log.Printf("downstream auth: %+v", try)
			defer close(try.reply)
			reply, err := chal(try.user, try.instruction, try.questions, try.echos)
			if err != nil {
				log.Printf("server chal: %v", err)
			}
			log.Printf("got reply from downstream: %v", reply)
			try.reply <- reply
		}
		if err := <-upstreamConnected; err != nil {
			log.Fatalf("upstream not connected: %v", err)
		}
		return nil, nil
	}
	upstreamChannel := make(chan *ssh.Client)
	go func() {
		upstreamConf.User = <-userChan
		defer close(upstreamChannel)
		defer close(authKBI)
		upstream, err := ssh.Dial("tcp", target, upstreamConf)
		if err != nil {
			upstreamConnected <- err
			log.Fatalf("upstream dial: %v", err)
		}
		log.Printf("upstream is connected")
		upstreamChannel <- upstream
	}()

	return upstreamChannel
}