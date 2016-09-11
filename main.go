package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	PORT = 3030

	// DNS query classes
	IN  = 1
	ANY = 255

	// DNS header flags
	FLG_RESPONSE          = 1 << 15
	FLG_RECURSION_DESIRED = 1 << 8

	FLG_RCODE_NO_ERROR       = 0
	FLG_RCODE_FORMAT_ERROR   = 1
	FLG_RCODE_SERVER_FAILURE = 2
	FLG_RCODE_NAME_ERROR     = 3
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

func serve(b []byte, s *net.UDPAddr, c *net.UDPConn) {
	buf := bytes.NewReader(b)

	var qHdr DNSQueryHdrPkt
	err := binary.Read(buf, binary.BigEndian, &qHdr)
	if err != nil {
		fmt.Println(err)
		return
	}
	// TODO: Sanity check header values

	queries := make([]Query, 0, qHdr.NQueries)
	for i := 0; i < int(qHdr.NQueries); i++ {
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

		// if q.pkt.QClass != IN && q.pkt.QClass != ANY {
		// 	continue
		// }

		queries = append(queries, q)
	}
	fmt.Printf("Q: %+v\n%v\n", qHdr, queries)
	fmt.Printf("%s\n", hex.EncodeToString(b))

	// TODO: Lookup DNS information (aka Step 2, profit)
	// For now, respond with name error

	wBuf := new(bytes.Buffer)

	rHdr := DNSQueryHdrPkt{QueryID: qHdr.QueryID}
	rHdr.Flags = FLG_RESPONSE | FLG_RCODE_NAME_ERROR
	if rHdr.Flags&FLG_RECURSION_DESIRED == FLG_RECURSION_DESIRED {
		rHdr.Flags |= FLG_RECURSION_DESIRED
	}

	rHdr.NQueries = qHdr.NQueries
	err = binary.Write(wBuf, binary.BigEndian, rHdr)
	if err != nil {
		fmt.Println(err)
		return
	}

	for i := 0; i < len(queries); i++ {
		// Write out label
		for _, l := range queries[i].Labels {
			wBuf.WriteByte(byte(len(l)))
			wBuf.WriteString(l)
		}
		wBuf.WriteByte(0)

		// Write out type and class
		binary.Write(wBuf, binary.BigEndian, queries[i].pkt)
	}

	fmt.Printf("A: %+v\n", wBuf.Bytes())
	fmt.Printf("%s\n", hex.EncodeToString(wBuf.Bytes()))

	ob := wBuf.Bytes()

	var n int
	if n, err = c.WriteToUDP(ob, s); n != len(ob) || err != nil {
		fmt.Println("Error writing response: %s\n", err)
		return
	}
}

func readNameservers() []string {
	var file *os.File
	var err error
	if file, err = os.Open("/etc/resolv.conf"); err != nil {
		if !os.IsNotExist(err) {
			fmt.Println("Error opening /etc/resolv.conf: %v", err)
		}
		return []string{}
	}
	defer file.Close()

	nameservers := make([]string, 0)
	s := bufio.NewScanner(file)
	for s.Scan() {
		l := s.Text()
		fields := strings.Fields(l)
		if len(fields) < 1 {
			continue
		}

		switch fields[0] {
		case "nameserver":
			addr := fields[1]
			// Paranoia - only take IP addresses and not host names
			if ip := net.ParseIP(addr); ip != nil {
				nameservers = append(nameservers, addr)
			}
		}
	}

	return nameservers
}

func main() {
	_ = readNameservers()

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

		go serve(b[:n], s, c)
	}
}
