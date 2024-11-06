// Package soap provides a SOAP HTTP client.
package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"

	"golang.org/x/net/html/charset"
)

// XSINamespace is a link to the XML Schema instance namespace.
const XSINamespace = "http://www.w3.org/2001/XMLSchema-instance"

var xmlTyperType reflect.Type = reflect.TypeOf((*XMLTyper)(nil)).Elem()

// A RoundTripper executes a request passing the given req as the SOAP
// envelope body. The HTTP response is then de-serialized onto the resp
// object. Returns error in case an error occurs serializing req, making
// the HTTP request, or de-serializing the response.
type RoundTripper interface {
	RoundTrip(req, resp Message) error
	RoundTripSoap12(action string, req, resp Message) error
}

// Message is an opaque type used by the RoundTripper to carry XML
// documents for SOAP.
type Message any

// Header is an opaque type used as the SOAP Header element in requests.
type Header any

// AuthHeader is a Header to be encoded as the SOAP Header element in
// requests, to convey credentials for authentication.
type AuthHeader struct {
	Namespace string `xml:"xmlns:ns,attr"`
	Username  string `xml:"ns:username"`
	Password  string `xml:"ns:password"`
}

// Client is a SOAP client.
type Client struct {
	URL                    string               // URL of the server
	UserAgent              string               // User-Agent header will be added to each request
	Namespace              string               // SOAP Namespace
	URNamespace            string               // Uniform Resource Namespace
	ThisNamespace          string               // SOAP This-Namespace (tns)
	TNSAttr                string               // SOAP This-Namespace (tns)
	XSIAttr                string               // SOAP This-Namespace (xsi)
	ExcludeActionNamespace bool                 // Include Namespace to SOAP Action header
	Envelope               string               // Optional SOAP Envelope
	Header                 Header               // Optional SOAP Header
	ContentType            string               // Optional Content-Type (default text/xml)
	Config                 *http.Client         // Optional HTTP client
	Pre                    func(*http.Request)  // Optional hook to modify outbound requests
	Post                   func(*http.Response) // Optional hook to snoop inbound responses
	Ctx                    context.Context      // Optional variable to allow Context Tracking.
	UsedNameSpaces         map[string]string    // Optional map to store used namespaces
}

// XMLTyper is an abstract interface for types that can set an XML type.
type XMLTyper interface {
	SetXMLType()
}

func setXMLType(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	switch v.Type().Kind() {
	case reflect.Interface:
		setXMLType(v.Elem())
	case reflect.Ptr:
		if v.IsNil() {
			break
		}
		ok := v.Type().Implements(xmlTyperType)
		if ok {
			v.MethodByName("SetXMLType").Call(nil)
		}
		setXMLType(v.Elem())
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			setXMLType(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanAddr() {
				setXMLType(v.Field(i).Addr())
			} else {
				setXMLType(v.Field(i))
			}
		}
	}
}

func doRoundTrip(c *Client, setHeaders func(*http.Request), in, out Message) error {
	setXMLType(reflect.ValueOf(in))
	req := &Envelope{
		EnvelopeAttr: c.Envelope,
		URNAttr:      c.URNamespace,
		NSAttr:       c.Namespace,
		TNSAttr:      c.TNSAttr,
		XSIAttr:      c.XSIAttr,
		Header:       c.Header,
		Body:         in,
	}

	if req.EnvelopeAttr == "" {
		req.EnvelopeAttr = "http://schemas.xmlsoap.org/soap/envelope/"
	}
	if req.NSAttr == "" {
		req.NSAttr = c.URL
	}

	for k, v := range c.UsedNameSpaces {
		switch k {

		case "tns0":
			req.TNS0 = v
		case "tns1":
			req.TNS1 = v
		case "tns2":
			req.TNS2 = v
		case "tns3":
			req.TNS3 = v
		case "tns4":
			req.TNS4 = v
		case "tns5":
			req.TNS5 = v
		case "tns6":
			req.TNS6 = v
		case "tns7":
			req.TNS7 = v
		case "tns8":
			req.TNS8 = v
		case "tns9":
			req.TNS9 = v
		case "tns10":
			req.TNS10 = v
		case "tns11":
			req.TNN11 = v
		case "tns12":
			req.TNS12 = v
		case "tns13":
			req.TNS13 = v
		case "tns14":
			req.TNS14 = v
		}
	}

	var b bytes.Buffer
	err := xml.NewEncoder(&b).Encode(req)
	if err != nil {
		return err
	}
	cli := c.Config
	if cli == nil {
		cli = http.DefaultClient
	}
	r, err := http.NewRequest("POST", c.URL, &b)
	if err != nil {
		return err
	}
	setHeaders(r)
	if c.Pre != nil {
		c.Pre(r)
	}

	if c.Ctx != nil {
		r = r.WithContext(c.Ctx)
	}

	resp, err := cli.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if c.Post != nil {
		c.Post(resp)
	}
	if resp.StatusCode != http.StatusOK {
		// read only the first MiB of the body in error case
		limReader := io.LimitReader(resp.Body, 1024*1024)
		body, _ := ioutil.ReadAll(limReader)
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Msg:        string(body),
		}
	}

	marshalStructure := struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    Message
	}{Body: out}

	decoder := xml.NewDecoder(resp.Body)
	decoder.CharsetReader = charset.NewReaderLabel
	return decoder.Decode(&marshalStructure)
}

// RoundTrip implements the RoundTripper interface.
func (c *Client) RoundTrip(in, out Message) error {
	headerFunc := func(r *http.Request) {
		if c.UserAgent != "" {
			r.Header.Add("User-Agent", c.UserAgent)
		}
		var actionName, soapAction string
		if in != nil {
			soapAction = reflect.TypeOf(in).Elem().Name()
		}
		ct := c.ContentType
		if ct == "" {
			ct = "text/xml"
		}
		r.Header.Set("Content-Type", ct)
		if in != nil {
			if c.ExcludeActionNamespace {
				actionName = soapAction
			} else {
				actionName = fmt.Sprintf("%s/%s", c.Namespace, soapAction)
			}
			r.Header.Add("SOAPAction", actionName)
		}
	}
	return doRoundTrip(c, headerFunc, in, out)
}

// RoundTripWithAction implements the RoundTripper interface for SOAP clients
// that need to set the SOAPAction header.
func (c *Client) RoundTripWithAction(soapAction string, in, out Message) error {
	headerFunc := func(r *http.Request) {
		if c.UserAgent != "" {
			r.Header.Add("User-Agent", c.UserAgent)
		}
		var actionName string
		ct := c.ContentType
		if ct == "" {
			ct = "text/xml"
		}
		r.Header.Set("Content-Type", ct)
		if in != nil {
			if c.ExcludeActionNamespace {
				actionName = soapAction
			} else {
				actionName = fmt.Sprintf("%s/%s", c.Namespace, soapAction)
			}
			r.Header.Add("SOAPAction", actionName)
		}
	}
	return doRoundTrip(c, headerFunc, in, out)
}

// RoundTripSoap12 implements the RoundTripper interface for SOAP 1.2.
func (c *Client) RoundTripSoap12(action string, in, out Message) error {
	headerFunc := func(r *http.Request) {
		r.Header.Add("Content-Type", fmt.Sprintf("application/soap+xml; charset=utf-8; action=\"%s\"", action))
	}
	return doRoundTrip(c, headerFunc, in, out)
}

// HTTPError is detailed soap http error
type HTTPError struct {
	StatusCode int
	Status     string
	Msg        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%q: %q", e.Status, e.Msg)
}

// Envelope is a SOAP envelope.
type Envelope struct {
	XMLName      xml.Name `xml:"soapenv:Envelope"` // default name
	EnvelopeAttr string   `xml:"xmlns:soapenv,attr"`
	NSAttr       string   `xml:"xmlns,attr"` // use default names space
	TNSAttr      string   `xml:"xmlns:tns,attr,omitempty"`
	URNAttr      string   `xml:"xmlns:urn,attr,omitempty"`
	XSIAttr      string   `xml:"xmlns:xsi,attr,omitempty"`
	Header       Message  `xml:"soapenv:Header"`
	Body         Message  `xml:"soapenv:Body"`

	TNS0  string `xml:"xmlns:tns0,attr,omitempty"`
	TNS1  string `xml:"xmlns:tns1,attr,omitempty"`
	TNS2  string `xml:"xmlns:tns2,attr,omitempty"`
	TNS3  string `xml:"xmlns:tns3,attr,omitempty"`
	TNS4  string `xml:"xmlns:tns4,attr,omitempty"`
	TNS5  string `xml:"xmlns:tns5,attr,omitempty"`
	TNS6  string `xml:"xmlns:tns6,attr,omitempty"`
	TNS7  string `xml:"xmlns:tns7,attr,omitempty"`
	TNS8  string `xml:"xmlns:tns8,attr,omitempty"`
	TNS9  string `xml:"xmlns:tns9,attr,omitempty"`
	TNS10 string `xml:"xmlns:tns10,attr,omitempty"`
	TNN11 string `xml:"xmlns:tns11,attr,omitempty"`
	TNS12 string `xml:"xmlns:tns12,attr,omitempty"`
	TNS13 string `xml:"xmlns:tns13,attr,omitempty"`
	TNS14 string `xml:"xmlns:tns14,attr,omitempty"`
}
