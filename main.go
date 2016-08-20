package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
)

const (
	PORT = 3030

	// DNS query classes
	IN  = 1
	ANY = 255
)

type DNSQueryHdrPkt struct {
	QueryID   uint16
	Flags     uint16
	NQueries  uint16
	NAnswers  uint16
	NAuthRecs uint16
	NAddRecs  uint16
}

type QuestionPkt struct {
	QType  uint16
	QClass uint16
}

type Query struct {
	Labels []string
	pkt    QuestionPkt
}

func serve(b []byte, _ *net.UDPAddr) {
	buf := bytes.NewReader(b)

	var hdr DNSQueryHdrPkt
	err := binary.Read(buf, binary.BigEndian, &hdr)
	if err != nil {
		fmt.Println(err)
		return
	}
	// TODO: Sanity check header values

	queries := make([]Query, 0, hdr.NQueries)
	for i := 0; i < int(hdr.NQueries); i++ {
		q := Query{}
		// Extract query name
		for {
			l, _ := buf.ReadByte()
			// TODO: Assumes success

			if l == 0 {
				break
			}

			s := make([]byte, l)
			_, _ = buf.Read(s)
			// TODO: Assumes success

			q.Labels = append(q.Labels, string(s))
		}
		// Query class & type
		if err = binary.Read(buf, binary.BigEndian, &q.pkt); err != nil {
			fmt.Println(err)
			return
		}

		if q.pkt.QClass != IN && q.pkt.QClass != ANY {
			continue
		}

		queries = append(queries, q)
	}
	fmt.Printf("%+v\n%v\n", hdr, queries)
}

func main() {
	fmt.Println("UDP listen on port", PORT)

	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", PORT))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	c, err := net.ListenUDP("udp", udpAddr)
	defer c.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	b := make([]byte, 1024)
	for {
		n, s, err := c.ReadFromUDP(b)
		if err != nil {
			fmt.Printf("err %v\n", err)
			continue
		}

		go serve(b[:n], s)
	}
}
