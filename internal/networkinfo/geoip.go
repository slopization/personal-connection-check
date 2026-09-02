package networkinfo

import (
	"github.com/oschwald/geoip2-golang"
	"net"
	"strconv"
)

type Geo struct{ city, asn *geoip2.Reader }

func Open(cityPath, asnPath string) *Geo {
	g := &Geo{}
	if cityPath != "" {
		if r, e := geoip2.Open(cityPath); e == nil {
			g.city = r
		}
	}
	if asnPath != "" {
		if r, e := geoip2.Open(asnPath); e == nil {
			g.asn = r
		}
	}
	return g
}
func (g *Geo) Close() {
	if g.city != nil {
		g.city.Close()
	}
	if g.asn != nil {
		g.asn.Close()
	}
}
func (g *Geo) Lookup(ip net.IP) Info {
	x := Info{IP: ip.String()}
	if g == nil {
		return x
	}
	if g.city != nil {
		if r, e := g.city.City(ip); e == nil {
			x.City = r.City.Names["en"]
			x.Country = r.Country.Names["en"]
		}
	}
	if g.asn != nil {
		if r, e := g.asn.ASN(ip); e == nil {
			x.ASN = fmtASN(r.AutonomousSystemNumber)
			x.ISP = r.AutonomousSystemOrganization
		}
	}
	return x
}
func fmtASN(n uint) string {
	if n == 0 {
		return ""
	}
	return "AS" + strconv.FormatUint(uint64(n), 10)
}
