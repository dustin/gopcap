// Interface to both live and offline pcap parsing.
package pcap

/*
#cgo LDFLAGS: -lpcap
#include <stdlib.h>
#include <pcap.h>

// Workaround for not knowing how to cast to const u_char**
int hack_pcap_next_ex(pcap_t *p, struct pcap_pkthdr **pkt_header,
                      u_char **pkt_data) {
    return pcap_next_ex(p, pkt_header, (const u_char **)pkt_data);
}
*/
import "C"
import (
	"errors"
	"net"
	"syscall"
	"unsafe"
)

type Pcap struct {
	cptr *C.pcap_t
}

type Stat struct {
	PacketsReceived  uint32
	PacketsDropped   uint32
	PacketsIfDropped uint32
}

type Interface struct {
	Name        string
	Description string
	Addresses   []IFAddress
	// TODO: add more elements
}

type IFAddress struct {
	IP      net.IP
	Netmask net.IPMask
	// TODO: add broadcast + PtP dst ?
}

func Openlive(device string, snaplen int32, promisc bool, timeout_ms int32) (handle *Pcap, err error) {
	var buf *C.char
	buf = (*C.char)(C.calloc(ERRBUF_SIZE, 1))
	h := new(Pcap)
	var pro int32
	if promisc {
		pro = 1
	} else {
		pro = 0
	}

	dev := C.CString(device)
	defer C.free(unsafe.Pointer(dev))

	h.cptr = C.pcap_open_live(dev, C.int(snaplen), C.int(pro), C.int(timeout_ms), buf)
	if nil == h.cptr {
		handle = nil
		err = errors.New(C.GoString(buf))
	} else {
		handle = h
	}
	C.free(unsafe.Pointer(buf))
	return
}

func Openoffline(file string) (handle *Pcap, err error) {
	var buf *C.char
	buf = (*C.char)(C.calloc(ERRBUF_SIZE, 1))
	h := new(Pcap)

	cf := C.CString(file)
	defer C.free(unsafe.Pointer(cf))

	h.cptr = C.pcap_open_offline(cf, buf)
	if nil == h.cptr {
		handle = nil
		err = errors.New(C.GoString(buf))
	} else {
		handle = h
	}
	C.free(unsafe.Pointer(buf))
	return
}

func (p *Pcap) Next() (pkt *Packet) {
	rv, _ := p.NextEx()

	return rv
}

func (p *Pcap) NextEx() (pkt *Packet, result int32) {
	var pkthdr_ptr *C.struct_pcap_pkthdr
	var pkthdr C.struct_pcap_pkthdr

	var buf_ptr *C.u_char
	var buf unsafe.Pointer
	result = int32(C.hack_pcap_next_ex(p.cptr, &pkthdr_ptr, &buf_ptr))

	buf = unsafe.Pointer(buf_ptr)
	pkthdr = *pkthdr_ptr

	if nil == buf {
		pkt = nil
		return
	}
	pkt = new(Packet)
	pkt.Time.Sec = int32(pkthdr.ts.tv_sec)
	pkt.Time.Usec = int32(pkthdr.ts.tv_usec)
	pkt.Caplen = uint32(pkthdr.caplen)
	pkt.Len = uint32(pkthdr.len)
	pkt.Data = make([]byte, pkthdr.caplen)

	for i := uint32(0); i < pkt.Caplen; i++ {
		pkt.Data[i] = *(*byte)(unsafe.Pointer(uintptr(buf) + uintptr(i)))
	}

	return
}

func (p *Pcap) Close() {
	C.pcap_close(p.cptr)
}

func (p *Pcap) Geterror() error {
	return errors.New(C.GoString(C.pcap_geterr(p.cptr)))
}

func (p *Pcap) Getstats() (stat *Stat, err error) {
	var cstats _Ctype_struct_pcap_stat
	if -1 == C.pcap_stats(p.cptr, &cstats) {
		return nil, p.Geterror()
	}

	stats := new(Stat)

	stats.PacketsReceived = uint32(cstats.ps_recv)
	stats.PacketsDropped = uint32(cstats.ps_drop)
	stats.PacketsIfDropped = uint32(cstats.ps_ifdrop)

	return stats, nil
}

func (p *Pcap) Setfilter(expr string) (err error) {
	var bpf _Ctype_struct_bpf_program

	cexpr := C.CString(expr)
	defer C.free(unsafe.Pointer(cexpr))

	if -1 == C.pcap_compile(p.cptr, &bpf, cexpr, 1, 0) {
		return p.Geterror()
	}

	if -1 == C.pcap_setfilter(p.cptr, &bpf) {
		C.pcap_freecode(&bpf)
		return p.Geterror()
	}

	C.pcap_freecode(&bpf)
	return nil
}

func Version() string {
	return C.GoString(C.pcap_lib_version())
}

func (p *Pcap) Datalink() int {
	return int(C.pcap_datalink(p.cptr))
}

func (p *Pcap) Setdatalink(dlt int) error {
	if -1 == C.pcap_set_datalink(p.cptr, C.int(dlt)) {
		return p.Geterror()
	}
	return nil
}

func DatalinkValueToName(dlt int) string {
	name := C.pcap_datalink_val_to_name(C.int(dlt))
	if nil != name {
		return C.GoString(name)
	}
	return ""
}

func DatalinkValueToDescription(dlt int) string {
	desc := C.pcap_datalink_val_to_description(C.int(dlt))
	if nil != desc {
		return C.GoString(desc)
	}
	return ""
}

func Findalldevs() (ifs []Interface, err error) {
	var buf *C.char
	buf = (*C.char)(C.calloc(ERRBUF_SIZE, 1))
	defer C.free(unsafe.Pointer(buf))
	var alldevsp *C.pcap_if_t

	if -1 == C.pcap_findalldevs((**C.pcap_if_t)(&alldevsp), buf) {
		return nil, errors.New(C.GoString(buf))
	}
	defer C.pcap_freealldevs((*C.pcap_if_t)(alldevsp))
	dev := alldevsp
	var i uint32
	for i = 0; dev != nil; dev = (*C.pcap_if_t)(dev.next) {
		i++
	}
	ifs = make([]Interface, i)
	dev = alldevsp
	for j := uint32(0); dev != nil; dev = (*C.pcap_if_t)(dev.next) {
		var iface Interface
		iface.Name = C.GoString(dev.name)
		iface.Description = C.GoString(dev.description)
		iface.Addresses = findalladdresses(dev.addresses)
		// TODO: add more elements
		ifs[j] = iface
		j++
	}
	return
}

func findalladdresses(addresses *_Ctype_struct_pcap_addr) (retval []IFAddress) {
	// TODO - make it support more than IPv4 and IPv6?
	retval = make([]IFAddress, 0, 1)
	for curaddr := addresses; curaddr != nil; curaddr = (*_Ctype_struct_pcap_addr)(curaddr.next) {
		var a IFAddress
		var err error
		a.IP, err = sockaddr_to_IP((*syscall.RawSockaddr)(unsafe.Pointer(curaddr.addr)))
		if err != nil {
			continue
		}
		a.Netmask, err = sockaddr_to_IP((*syscall.RawSockaddr)(unsafe.Pointer(curaddr.addr)))
		if err != nil {
			continue
		}
		retval = append(retval, a)
	}
	return
}

func sockaddr_to_IP(rsa *syscall.RawSockaddr) (IP []byte, err error) {
	switch rsa.Family {
	case syscall.AF_INET:
		pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(rsa))
		IP = make([]byte, 4)
		for i := 0; i < len(IP); i++ {
			IP[i] = pp.Addr[i]
		}
		return
	case syscall.AF_INET6:
		pp := (*syscall.RawSockaddrInet6)(unsafe.Pointer(rsa))
		IP = make([]byte, 16)
		for i := 0; i < len(IP); i++ {
			IP[i] = pp.Addr[i]
		}
		return
	}
	err = errors.New("Unsupported address type")
	return
}

func (p *Pcap) Inject(data []byte) (err error) {
	buf := (*C.char)(C.malloc((C.size_t)(len(data))))

	for i := 0; i < len(data); i++ {
		*(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(buf)) + uintptr(i))) = data[i]
	}

	if -1 == C.pcap_inject(p.cptr, unsafe.Pointer(buf), (C.size_t)(len(data))) {
		err = p.Geterror()
	}
	C.free(unsafe.Pointer(buf))
	return
}
