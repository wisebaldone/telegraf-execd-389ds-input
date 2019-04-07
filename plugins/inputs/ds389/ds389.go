package ds389

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/influxdata/telegraf/internal/tls"
	ldap "gopkg.in/ldap.v2"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type ds389 struct {
	Host               string
	Port               int
	SSL                string `toml:"ssl"` // Deprecated in 1.7; use TLS
	TLS                string `toml:"tls"`
	InsecureSkipVerify bool
	SSLCA              string `toml:"ssl_ca"` // Deprecated in 1.7; use TLSCA
	TLSCA              string `toml:"tls_ca"`
	BindDn             string
	BindPassword       string
	Dbtomonitor        []string
}

const sampleConfig string = `
  host = "ldap_instance"
  port = 389

  # ldaps, starttls, or no encryption. default is an empty string, disabling all encryption.
  # note that port will likely need to be changed to 636 for ldaps
  # valid options: "" | "starttls" | "ldaps"
  tls = ""

  # skip peer certificate verification. Default is false.
  insecure_skip_verify = false

  # Path to PEM-encoded Root certificate to use to verify server certificate
  tls_ca = "/etc/ssl/certs.pem"

  # dn/password to bind with. If bind_dn is empty, an anonymous bind is performed.
  bind_dn = ""
  bind_password = ""

  #Gather dbname monitor
  # Comma separated list of db filename
  # dbtomonitor = ["exampleDB"]
`

var searchMonitor = "cn=Monitor"
var searchLdbmMonitor = "cn=monitor,cn=ldbm database,cn=plugins,cn=config"

var searchFilter = "(objectClass=*)"
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
	return "ds389 cn=Monitor plugin"
}

// return an initialized ds389
func Newds389() *ds389 {
	return &ds389{
		Host:               "localhost",
		Port:               389,
		SSL:                "",
		TLS:                "",
		InsecureSkipVerify: false,
		SSLCA:              "",
		TLSCA:              "",
		BindDn:             "",
		BindPassword:       "",
		Dbtomonitor:        []string{},
	}
}

// gather metrics
func (o *ds389) Gather(acc telegraf.Accumulator) error {
	if o.TLS == "" {
		o.TLS = o.SSL
	}
	if o.TLSCA == "" {
		o.TLSCA = o.SSLCA
	}

	var err error
	var l *ldap.Conn
	if o.TLS != "" {
		// build tls config
		clientTLSConfig := tls.ClientConfig{
			TLSCA:              o.TLSCA,
			InsecureSkipVerify: o.InsecureSkipVerify,
		}
		tlsConfig, err := clientTLSConfig.TLSConfig()
		if err != nil {
			acc.AddError(err)
			return nil
		}
		if o.TLS == "ldaps" {
			l, err = ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", o.Host, o.Port), tlsConfig)
			if err != nil {
				acc.AddError(err)
				return nil
			}
		} else if o.TLS == "starttls" {
			l, err = ldap.Dial("tcp", fmt.Sprintf("%s:%d", o.Host, o.Port))
			if err != nil {
				acc.AddError(err)
				return nil
			}
			err = l.StartTLS(tlsConfig)
		} else {
			acc.AddError(fmt.Errorf("Invalid setting for ssl: %s", o.TLS))
			return nil
		}
	} else {
		l, err = ldap.Dial("tcp", fmt.Sprintf("%s:%d", o.Host, o.Port))
	}

	if err != nil {
		acc.AddError(err)
		return nil
	}
	defer l.Close()

	// username/password bind
	if o.BindDn != "" && o.BindPassword != "" {
		err = l.Bind(o.BindDn, o.BindPassword)
		if err != nil {
			acc.AddError(err)
			return nil
		}
	}

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
		acc.AddError(err)
		return nil
	}

	gatherSearchResult(sr, o, acc)

	searchLdbmRequest := ldap.NewSearchRequest(
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

	sldbmr, err := l.Search(searchLdbmRequest)

	if err != nil {
		acc.AddError(err)
		return nil
	}

	gatherSearchResult(sldbmr, o, acc)

	if len(o.Dbtomonitor) > 0 {
		for _, db := range o.Dbtomonitor {
			var searchDbMonitor = fmt.Sprintf("cn=monitor,cn=%s,cn=ldbm database,cn=plugins,cn=config", db)
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
				acc.AddError(err)
				return nil
			}
			gatherDbSearchResult(sdbr, o, acc, db)
		}
	}
	return nil
}

func gatherSearchResult(sr *ldap.SearchResult, o *ds389, acc telegraf.Accumulator) {
	fields := map[string]interface{}{}
	tags := map[string]string{
		"server": o.Host,
		"port":   strconv.Itoa(o.Port),
	}
	for _, entry := range sr.Entries {
		for _, attr := range entry.Attributes {
			if len(attr.Values[0]) >= 1 {
				if v, err := strconv.ParseInt(attr.Values[0], 10, 64); err == nil {
					//fmt.Println(attr.Name, v)
					fields[attr.Name] = v
				}
			}
		}
	}
	acc.AddFields("ds389", fields, tags)
	return
}

func gatherDbSearchResult(sr *ldap.SearchResult, o *ds389, acc telegraf.Accumulator, dbname string) {
	fields := map[string]interface{}{}
	tags := map[string]string{
		"server": o.Host,
		"port":   strconv.Itoa(o.Port),
	}
	for _, entry := range sr.Entries {
		for _, attr := range entry.Attributes {
			if len(attr.Values[0]) >= 1 {
				if v, err := strconv.ParseInt(attr.Values[0], 10, 64); err == nil {
					attrName := fmt.Sprint(strings.ToLower(dbname), "_", attr.Name)
					//fmt.Println(attrName, v)
					fields[attrName] = v
				}
			}
		}
	}
	acc.AddFields("ds389", fields, tags)
	return
}

func init() {
	inputs.Add("ds389", func() telegraf.Input { return Newds389() })
}