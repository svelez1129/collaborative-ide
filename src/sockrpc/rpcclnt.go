package sockrpc

import (
	"log"
	"net"
	"time"

	"github.com/svelez1129/collaborative-ide/src/labrpc"
	"github.com/svelez1129/collaborative-ide/src/tester1/demux"
)

type RPCArgs struct {
	Method string
	Args   []byte
}

type RPCClnt struct {
	clnt   string
	srvEnd string
	dmx    *demux.DemuxClnt
}

func NewRPCClnt(clntEnd, srvEnd string) *RPCClnt {
	return &RPCClnt{clnt: clntEnd, dmx: getDmx(clntEnd, srvEnd), srvEnd: srvEnd}
}

// TryNewRPCClnt attempts a single connection with a short timeout.
// Returns nil if the peer is not yet listening — the caller should retry later.
func TryNewRPCClnt(clntEnd, srvEnd string) *RPCClnt {
	c, err := net.DialTimeout("unix", SockName(srvEnd), 200*time.Millisecond)
	if err != nil {
		return nil
	}
	t := demux.NewTransport(c)
	dc, err := demux.NewDemuxClnt(clntEnd, srvEnd, t)
	if err != nil {
		c.Close()
		return nil
	}
	return &RPCClnt{clnt: clntEnd, dmx: dc, srvEnd: srvEnd}
}

func (rpcc *RPCClnt) Server() string {
	return rpcc.srvEnd
}

func (rpcc *RPCClnt) Close() {
	rpcc.dmx.Close()
}

func (rpcc *RPCClnt) RPC(method string, args []byte) ([]byte, bool) {
	//log.Printf("RPC to srv %q m %v l %d", rpcc.srvEnd, method, len(args))
	req := &RPCArgs{Method: method, Args: args}
	b := labrpc.Marshall(req)
	rep, ok, err := rpcc.dmx.SendReceive(b)
	if err != nil {
		return nil, false
	}
	return rep, ok
}

func (rpcc *RPCClnt) RPCMarshall(method string, args any, reply any) bool {
	rep, ok := rpcc.RPC(method, labrpc.Marshall(args))
	if !ok {
		return ok
	}
	labrpc.Unmarshall(rep, reply)
	return ok
}

func dial(srvEnd string) (net.Conn, error) {
	const MAXRETRY = 100
	var r error

	for i := 0; i < MAXRETRY; i++ {
		c, err := net.Dial("unix", SockName(srvEnd))
		if err == nil {
			return c, nil
		}
		r = err
		time.Sleep(100 * time.Millisecond)
	}
	return nil, r
}

// Get a connection to sock
func getDmx(clntEnd, srvEnd string) *demux.DemuxClnt {
	c, err := dial(srvEnd)
	if err != nil {
		log.Fatalf("%v: Dial %v err %v", clntEnd, srvEnd, err)
	}
	if t := demux.NewTransport(c); err != nil {
		log.Fatalf("%v: NewTransport err %v", clntEnd, err)
		return nil
	} else {
		dc, err := demux.NewDemuxClnt(clntEnd, srvEnd, t)
		if err != nil {
			log.Fatalf("%v: NewDemuxClnt err %v", clntEnd, err)
		}
		return dc
	}
}
