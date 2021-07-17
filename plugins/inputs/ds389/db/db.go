package db

import (
	"fmt"
	ldap "github.com/go-ldap/ldap/v3"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/common/tls"
	"github.com/influxdata/telegraf/plugins/inputs"
	"github.com/thoas/go-funk"
	"regexp"
	"strconv"
	"strings"
)

//ds389 389 DS object
type ds389 struct {
	Url                string
	Starttls           bool
	InsecureSkipVerify bool
	BindDn             string `toml:"bind_dn"`
	BindPassword       string `toml:"bind_password"`
	Dbtomonitor        []string
	AllDbmonitor       bool
	Status             bool
	tls.ClientConfig
}

type attributeNamer func(name string) string

const sampleConfig string = `

  # LDAP Server URL ( Support protocols are ldap:\\, ldaps:\\, ldapi:\\ )
  url = "ldap://hostname:port"

  # Enable starttls encryption for plain ldap:\\ URLs, is not needed for ldaps:\\
  starttls = true
  
  ## Optional TLS Config
  # tls_ca = "/etc/telegraf/ca.pem"
  # tls_cert = "/etc/telegraf/cert.pem"
  # tls_key = "/etc/telegraf/key.pem"
  ## Use TLS but skip chain & host verification
  insecure_skip_verify = false

  # dn/password to bind with. If bind_dn is empty, an anonymous bind is performed.
  bind_dn = ""
  bind_password = ""

  ## Gather dbname to monitor
  # Comma separated list of db filename
  # dbtomonitor = ["exampleDB"]
  # If true, alldbmonitor monitors all db and overrides "dbtomonitor".
  alldbmonitor = false

  # Connections status monitor
  status = false
`

const searchMonitor = "cn=Monitor"
const searchLdbmMonitor = "cn=monitor,cn=ldbm database,cn=plugins,cn=config"

var searchFilter = "(objectClass=extensibleObject)"
var searchAttrs = []string{
	"currentconnections",
	"totalconnections",
	"currentconnectionsatmaxthreads",
	"maxthreadsperconnhits",
	"dtablesize",
	"readwaiters",
	"opsinitiated",
	"opscompleted",
	"entriessent",
	"bytessent",
	"anonymousbinds",
	"unauthbinds",
	"simpleauthbinds",
	"strongauthbinds",
	"bindsecurityerrors",
	"inops",
	"readops",
	"compareops",
	"addentryops",
	"removeentryops",
	"modifyentryops",
	"modifyrdnops",
	"listops",
	"searchops",
	"onelevelsearchops",
	"wholesubtreesearchops",
	"referrals",
	"chainings",
	"securityerrors",
	"errors",
	"connections",
	"connectionseq",
	"connectionsinmaxthreads",
	"connectionsmaxthreadscount",
	"bytesrecv",
	"bytessent",
	"entriesreturned",
	"referralsreturned",
	"masterentries",
	"copyentries",
	"cacheentries",
	"cachehits",
	"slavehits",
	"backendmonitordn",
	"connection",
	"version",
}

var searchLdbmAttrs = []string{
	"dbcachehitratio",
	"dbcachehits",
	"dbcachepagein",
	"dbcachepageout",
	"dbcacheroevict",
	"dbcacherwevict",
	"dbcachetries",
}

var searchDbAttrs = []string{}

func (o *ds389) SampleConfig() string {
	return sampleConfig
}

func (o *ds389) Description() string {
	return "Gather 389 directory server metrics from cn=Monitor,cn=*,ldbm database,cn=plugins,cn=config"
}

//NewDS389 set default value in order to initilize a connection
func NewDs389() *ds389 {
	return &ds389{
		Url:                "ldap://localhost:389",
		Starttls:           true,
		InsecureSkipVerify: false,
		BindDn:             "cn=Directory Manager",
		BindPassword:       "secret",
		Dbtomonitor:        []string{"userRoot"},
		AllDbmonitor:       false,
		Status:             false,
	}
}

/*
	connect is used to connect and bind to a 389ds server.
 */
func (o *ds389) connect() (*ldap.Conn, error) {
	clientTLSConfig := tls.ClientConfig{
		TLSCA:              o.TLSCA,
		InsecureSkipVerify: o.InsecureSkipVerify,
	}
	tlsConfig, err := clientTLSConfig.TLSConfig()
	if err != nil {
		return nil, err
	}

	l, err := ldap.DialURL(o.Url, ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, err
	}

	if o.Starttls {
		err = l.StartTLS(tlsConfig)
		if err != nil {
			l.Close()
			return nil, err
		}
	}

	// username/password bind
	if o.BindDn != "" && o.BindPassword != "" {
		err = l.Bind(o.BindDn, o.BindPassword)
		if err != nil {
			l.Close()

			return nil, err
		}
	}

	return l, nil
}

func (o *ds389) gatherBaseStats(l *ldap.Conn) (map[string]interface{}, error) {

	search := ldap.NewSearchRequest(
		searchMonitor,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		searchFilter,
		searchAttrs,
		nil,
	)

	results, err := l.Search(search)
	if err != nil {
		return nil, err
	}

	stats := flatternSearchEntry(results.Entries, func(name string) string {
		return name
	})

	return stats, nil
}

func (o *ds389) gatherLDBMStats(l *ldap.Conn) (map[string]interface{}, error) {
	search := ldap.NewSearchRequest(
		searchLdbmMonitor,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		searchFilter,
		searchLdbmAttrs,
		nil,
	)

	results, err := l.Search(search)
	if err != nil {
		return nil, err
	}

	stats := flatternSearchEntry(results.Entries, func(name string) string {
		return name
	})

	return stats, nil
}

func (o *ds389) gatherDatabaseStats(l *ldap.Conn) (map[string]interface{}, error) {
	stats := map[string]interface{}{}

	searchRequest := ldap.NewSearchRequest(
		searchMonitor,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		searchFilter,
		searchAttrs,
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	var availableDbs []string
	for _, dn := range sr.Entries[0].GetAttributeValues("backendmonitordn") {
		r := regexp.MustCompile(`cn=monitor,cn=(?P<db>\w+),cn=ldbm database,cn=plugins,cn=config`)
		db := r.FindStringSubmatch(dn)[1]

		if !o.AllDbmonitor && !funk.Contains(o.Dbtomonitor, db) {
			// Skip as we dont want to monitor this DB
			continue
		}

		availableDbs = append(availableDbs, db)
	}

	for _, db := range availableDbs {
		searchDbMonitor := fmt.Sprintf("cn=monitor,cn=%s,cn=ldbm database,cn=plugins,cn=config", db)
		searchDbRequest := ldap.NewSearchRequest(
			searchDbMonitor,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0,
			0,
			false,
			searchFilter,
			searchDbAttrs,
			nil,
		)

		sdbr, err := l.Search(searchDbRequest)
		if err != nil {
			return nil, err
		}

		dbStats := flatternSearchEntry(sdbr.Entries, func(name string) string {
			return fmt.Sprint(strings.ToLower(db), "_", name)
		})

		stats = mergeMaps(stats, dbStats)
	}

	return stats, nil
}

func (o *ds389) gatherConnections(l *ldap.Conn) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// Gather metrics from 389 DS
func (o *ds389) Gather(acc telegraf.Accumulator) error {

	l, err := o.connect()
	if err != nil {
		acc.AddError(err)
		return err
	}
	defer l.Close()

	stats, err := o.gatherBaseStats(l)
	if err != nil {
		acc.AddError(err)
		return err
	}

	ldbmStats, err := o.gatherLDBMStats(l)
	if err != nil {
		acc.AddError(err)
		return err
	}
	stats = mergeMaps(stats, ldbmStats)

	dbStats, err := o.gatherDatabaseStats(l)
	if err != nil {
		acc.AddError(err)
		return err
	}
	stats = mergeMaps(stats, dbStats)

	if o.Status {
		connectionStats, err := o.gatherConnections(l)
		if err != nil {
			acc.AddError(err)
			return err
		}
		stats = mergeMaps(stats, connectionStats)
	}

	// Add metrics
	tags := map[string]string{
		"URL":  o.Url,
		//"version": sr.Entries[0].GetAttributeValue("version"),
	}
	acc.AddFields("ds389", stats, tags)
	return nil
}

func flatternSearchEntry(entries []*ldap.Entry, name attributeNamer) map[string]interface{} {
	fields := map[string]interface{}{}

	for _, entry := range entries {
		for _, attr := range entry.Attributes {
			if len(attr.Values) != 1 {
				continue
			}

			if v, err := strconv.ParseInt(attr.Values[0], 10, 64); err == nil {
				fields[name(attr.Name)] = v
			}
		}
	}

	return fields
}

func mergeMaps(a map[string]interface{}, b map[string]interface{}) map[string]interface{} {
	for k, v := range b {
		a[k] = v
	}

	return a
}

func init() {
	inputs.Add("ds389_db", func() telegraf.Input { return NewDs389() })
}
