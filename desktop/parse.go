package desktop

import (
	"fmt"
	"strings"
)

//SPEC: https://specifications.freedesktop.org/desktop-entry-spec/latest/ar01s03.html
type File struct {
	lines []any
}

type comment string

type group struct {
	Name  string
	lines any
	KV    kv
}

type kv map[string]string

func ParseEntry(content string) (*File, error) {
	f := File{}
	// Desktop entry files are encoded in UTF-8.
	// A file is interpreted as a series of lines that are separated by linefeed characters.
	// Case is significant everywhere in the file.

	textlines := strings.Split(content, "\n")

	for len(textlines) != 0 {
		//the spec isn't super clear here on leading or trailing whitespace
		//but it would not make much sense to fail a parse because of a space before a "#" or a trailing space on a group header
		line := strings.TrimSpace(textlines[0])
		/*
			Comments
			Lines beginning with a # and blank lines are considered comments and will be ignored, however they should be preserved across reads and writes of the desktop entry file.

			Comment lines are uninterpreted and may contain any character (except for LF). However, using UTF-8 for comment lines that contain characters not in ASCII is encouraged.
		*/
		if strings.HasPrefix(line, "#") || line == "" { //already split by LF, so would be empty
			//store the original textline here, to keep whitespace intact
			f.lines = append(f.lines, comment(textlines[0]))
			textlines = textlines[1:]
			continue
		}
		/*
			Group headers
			A group header with name groupname is a line in the format:

			[groupname]
			Group names may contain all ASCII characters except for [ and ] and control characters.

			Multiple groups may not have the same name.

			All {key,value} pairs following a group header until a new group header belong to the group.

			The basic format of the desktop entry file requires that there be a group header named Desktop Entry. There may be other groups present in the file, but this is the most important group which explicitly needs to be supported. This group should also be used as the "magic key" for automatic MIME type detection. There should be nothing preceding this group in the desktop entry file but possibly one or more comments.
		*/
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			grp := group{}
			grp.Name = line[1 : len(line)-1]
			grp.KV = make(kv, 0)
			textlines = textlines[1:]
			for len(textlines) != 0 {
				line := strings.TrimSpace(textlines[0])
				/*
					Entries
					Entries in the file are {key,value} pairs in the format:

					Key=Value
					Space before and after the equals sign should be ignored; the = sign is the actual delimiter.

					Only the characters A-Za-z0-9- may be used in key names.

					As the case is significant, the keys Name and NAME are not equivalent.

					Multiple keys in the same group may not have the same name. Keys in different groups may have the same name.
				*/

				// duplicating logic here in the nested loop is kinda ugly, but whatever, it's only a few lines

				if strings.HasPrefix(line, "#") || line == "" { //already split by LF, so would be empty
					f.lines = append(f.lines, comment(textlines[0]))
					textlines = textlines[1:]
					continue
				}

				if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
					break
				}
				key, value, found := strings.Cut(line, "=")
				if !found {
					return nil, fmt.Errorf("unable to parse line:\n%s\nExpected key value pair separated by '=', but none found", line)
				}
				cleankey := strings.TrimSpace(key)
				cleanvalue := strings.TrimSpace(value)
				if _, exists := grp.KV[cleankey]; exists {
					return nil, fmt.Errorf("duplicate key '%s' in section '%s'", cleankey, grp.Name)
				}
				grp.KV[cleankey] = cleanvalue
				textlines = textlines[1:]
			}
			f.lines = append(f.lines, grp)
		}

	}
	return &f, nil
}

func (f *File) String() string {
	var sb strings.Builder
	for _, line := range f.lines {
		switch o := line.(type) {
		case comment:
			sb.WriteString(string(o))
		case group:
			sb.WriteRune('[')
			sb.WriteString(o.Name)
			sb.WriteRune(']')
			sb.WriteRune('\n')
			for k, v := range o.KV {
				sb.WriteString(k)
				sb.WriteRune('=')
				sb.WriteString(v)
				sb.WriteRune('\n')
			}
		default:
			panic("should never happen")
		}
		sb.WriteRune('\n')
	}

	return sb.String()
}

func (f *File) Groups() []*group {
	var res []*group
	for _, line := range f.lines {
		if grp, ok := line.(group); ok {
			res = append(res, &grp)
		}
	}
	return res
}

func (f *File) Group(s string) (group *group, found bool) {
	groups := f.Groups()
	for _, grp := range groups {
		if grp.Name == s {
			return grp, true
		}
	}
	return nil, false
}
