package appstream

import (
	"encoding/xml"
	"io"
	"io/ioutil"
)

//SPEC: https://www.freedesktop.org/wiki/Distributions/AppStream/

// struct generated with https://www.onlinetool.io/xmltogo/
type Component struct {
	XMLName         xml.Name `xml:"component"`
	Text            string   `xml:",chardata"`
	Type            string   `xml:"type,attr"`
	ID              string   `xml:"id"`
	MetadataLicense string   `xml:"metadata_license"`
	ProjectLicense  string   `xml:"project_license"`
	Name            string   `xml:"name"`
	URL             struct {
		Text string `xml:",chardata"`
		Type string `xml:"type,attr"`
	} `xml:"url"`
	Summary     string `xml:"summary"`
	Description struct {
		Text string `xml:",chardata"`
		P    string `xml:"p"`
	} `xml:"description"`
	Screenshots struct {
		Text       string `xml:",chardata"`
		Screenshot struct {
			Text    string `xml:",chardata"`
			Type    string `xml:"type,attr"`
			Image   string `xml:"image"`
			Caption string `xml:"caption"`
		} `xml:"screenshot"`
	} `xml:"screenshots"`
}

func Parse(r io.Reader) (*Component, error) {
	comp := Component{}

	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	err = xml.Unmarshal(buf, &comp)

	return &comp, nil
}
