package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
)

const (
	soapAddress   = "https://bugs.debian.org/cgi-bin/soap.cgi"
	soapNamespace = "Debbugs/SOAP"
)

type patch struct {
	Author  string
	Subject string
	Data    []byte
}

func getMostRecentPatch(url, bug string) (patch, error) {
	var result patch
	// TODO: write a WSDL file and use a proper Go SOAP library? see https://golanglibs.com/top?q=soap
	req := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
  SOAP-ENV:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"
  xmlns:SOAP-ENC="http://schemas.xmlsoap.org/soap/encoding/"
  xmlns:xsi="http://www.w3.org/1999/XMLSchema-instance"
  xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:xsd="http://www.w3.org/1999/XMLSchema"
>
<SOAP-ENV:Body>
<ns1:get_bug_log xmlns:ns1="Debbugs/SOAP" SOAP-ENC:root="1">
<v1 xsi:type="xsd:int">%s</v1>
</ns1:get_bug_log>
</SOAP-ENV:Body>
</SOAP-ENV:Envelope>
`, bug)
	resp, err := http.Post(url, "", strings.NewReader(req))
	if err != nil {
		return result, err
	}
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		return result, fmt.Errorf("Unexpected HTTP status code: got %d, want %d", got, want)
	}

	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return result, err
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return result, fmt.Errorf("Unexpected Content-Type: got %q, want multipart/*", resp.Header.Get("Content-Type"))
	}

	var r struct {
		XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
		Bugs    []struct {
			XMLName xml.Name `xml:"Debbugs/SOAP item"`
			MsgNum  int      `xml:"msg_num"`
			Body    string   `xml:"body"`
			Header  string   `xml:"header"`
		} `xml:"Body>get_bug_logResponse>Array>item"`
	}

	if err := xml.NewDecoder(resp.Body).Decode(&r); err != nil {
		return result, err
	}

	// TODO: handle bugs with multiple messages
	if len(r.Bugs) != 1 {
		log.Fatal("len(r.Bugs) = %d", len(r.Bugs))
	}

	body := r.Bugs[0].Body
	mr := multipart.NewReader(strings.NewReader(body), params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		disposition, _, err := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if err != nil {
			log.Printf("Skipping MIME part with invalid Content-Disposition header (%v)", err)
			continue
		}
		// TODO: is disposition always lowercase?
		if got, want := disposition, "attachment"; got != want {
			log.Printf("Skipping MIME part with unexpected Content-Disposition: got %q, want %q", got, want)
			continue
		}

		if p.Header.Get("Content-Transfer-Encoding") == "base64" {
			m, err := mail.ReadMessage(strings.NewReader(r.Bugs[0].Header + body))
			if err != nil {
				return result, err
			}
			result.Author = m.Header.Get("From")
			result.Subject = m.Header.Get("Subject")
			encoded, err := ioutil.ReadAll(p)
			if err != nil {
				return result, err
			}
			result.Data, err = base64.StdEncoding.DecodeString(string(encoded))
			return result, err
		} else {
			log.Fatal("unsupported Content-Transfer-Encoding: %q", p.Header.Get("Content-Transfer-Encoding"))
		}
	}

	return result, fmt.Errorf("No MIME part with Content-Disposition == attachment found")
}
