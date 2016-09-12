package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
)

const (
	PORT = 3030 // Port to listen on
)

// DNS query classes
const (
	IN  = 1
	ANY = 255
)

// DNS header flags
const (
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

type questionPkt struct {
	QType  uint16
	QClass uint16
}

type query struct {
	Labels []string
	pkt    questionPkt
}

func buildQueryPacket(id, flags uint16, queries []query) ([]byte, error) {
	hdr := DNSQueryHdrPkt{
		QueryID:  id,
		Flags:    flags,
		NQueries: uint16(len(queries)),
	}
	wBuf := new(bytes.Buffer)
	if err := binary.Write(wBuf, binary.BigEndian, hdr); err != nil {
		return []byte{}, err
	}

	for _, q := range queries {
		for _, l := range q.Labels {
			// Write out label
			wBuf.WriteByte(byte(len(l)))
			wBuf.WriteString(l)
		}
		wBuf.WriteByte(0)

		// Write out type and class
		binary.Write(wBuf, binary.BigEndian, q.pkt)
	}

	return wBuf.Bytes(), nil
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

	queries := make([]query, 0, qHdr.NQueries)
	for i := 0; i < int(qHdr.NQueries); i++ {
		q := query{}
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
	id := uint16(rand.Int31n(65536))
	upstream, err := buildQueryPacket(id, FLG_RECURSION_DESIRED, queries)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("UQ: %+v\n", upstream)
	fmt.Printf("%s\n", hex.EncodeToString(upstream))

	// For now, respond with name error
	aFlags := uint16(FLG_RESPONSE | FLG_RCODE_NAME_ERROR)
	if qHdr.Flags&FLG_RECURSION_DESIRED == FLG_RECURSION_DESIRED {
		aFlags |= FLG_RECURSION_DESIRED
	}
	answer, err := buildQueryPacket(qHdr.QueryID, aFlags, queries)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("A: %+v\n", answer)
	fmt.Printf("%s\n", hex.EncodeToString(answer))

	var n int
	if n, err = c.WriteToUDP(answer, s); n != len(answer) || err != nil {
		fmt.Println("Error writing response: %s\n", err)
		return
	}
}

func readNameservers(resolvconf string) []string {
	var file *os.File
	var err error
	if file, err = os.Open(resolvconf); err != nil {
		if !os.IsNotExist(err) {
			fmt.Println("Error opening /etc/resolv.conf: %v", err)
		}
		return []string{}
	}
	defer file.Close()

	var nameservers []string
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
	nameservers := readNameservers("/etc/resolv.conf")
	fmt.Printf("Found %d nameservers in /etc/resolv.conf\n", len(nameservers))

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
