package collect

import (
	"net"
	"strconv"
	"strings"
)

// RouteRow is one default route (0.0.0.0/0) as Windows reports it.
type RouteRow struct {
	IfIndex         int
	NextHop         string
	RouteMetric     int
	InterfaceMetric int
	Status          string
}

// PickDefaultRoute chooses the route Windows really uses. A route on an
// adapter that is not up is dropped, because Windows can keep an "Alive"
// route on an unplugged cable. Of the rest, the lowest route metric plus
// interface metric wins, and a tie goes to the lowest interface number.
// It reports false when no route is on an adapter that is up.
func PickDefaultRoute(rows []RouteRow) (RouteRow, bool) {
	var best RouteRow
	found := false
	for _, r := range rows {
		if r.Status != "Up" {
			continue
		}
		if !found || better(r, best) {
			best, found = r, true
		}
	}
	return best, found
}

// ChooseAdapterRoute picks the route whose adapter the check reads. A live
// route always wins (PickDefaultRoute). When none is live it reports false
// and does not guess between stale routes: the cable's lower metric would
// always win, so a Wi-Fi user whose Wi-Fi drops would be told the cable is
// at fault. The adapter script then falls back, and the diagnosis uses what
// the machine has (HasWired, HasWiFi, NoPhysicalUp).
func ChooseAdapterRoute(rows []RouteRow) (RouteRow, bool) {
	return PickDefaultRoute(rows)
}

// settingsScript fills the settings script with the interface the adapter
// was read from, so DNS comes from the same adapter. An index of 0 or less
// (no adapter read) becomes -1, which tells the script to choose the old way.
func settingsScript(ifIndex int) string {
	if ifIndex <= 0 {
		ifIndex = -1
	}
	return strings.ReplaceAll(scriptSettings, "%IFINDEX%", strconv.Itoa(ifIndex))
}

func better(a, b RouteRow) bool {
	sa, sb := a.RouteMetric+a.InterfaceMetric, b.RouteMetric+b.InterfaceMetric
	if sa != sb {
		return sa < sb
	}
	return a.IfIndex < b.IfIndex
}

// adapterScript fills the adapter script with the chosen route. Only an
// integer and a checked IPv4 address ever go into the script.
//   - ok: the chosen interface (see ChooseAdapterRoute) and its router.
//   - !ok but routesRead: no default route is on a live adapter, so index 0
//     and no router; the script falls back to the first physical adapter
//     that is up, else the first physical adapter.
//   - !routesRead: the route list could not be read, so index -1 tells the
//     script to choose the route itself, the old way.
func adapterScript(pick RouteRow, ok, routesRead bool) string {
	idx, gw := 0, ""
	switch {
	case !routesRead:
		idx = -1
	case ok && pick.IfIndex > 0:
		idx = pick.IfIndex
		// 0.0.0.0 (an on-link route), broadcast, multicast, loopback and
		// link-local are not a router to ping, so they count as no router.
		if ip := net.ParseIP(strings.TrimSpace(pick.NextHop)).To4(); ip != nil && ip.IsGlobalUnicast() && !ip.Equal(net.IPv4bcast) {
			gw = ip.String()
		}
	}
	s := strings.ReplaceAll(scriptAdapter, "%IFINDEX%", strconv.Itoa(idx))
	return strings.ReplaceAll(s, "%GATEWAY%", gw)
}
