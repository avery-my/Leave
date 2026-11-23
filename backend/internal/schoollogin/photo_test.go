package schoollogin

import "testing"

func TestParsePhotoID(t *testing.T) {
	for _, test := range []struct {
		name  string
		photo string
		want  string
	}{
		{name: "attachment JSON", photo: `[{"contentType":"image/jpeg","id":"abc123","originalFilename":"student.jpg","size":28191}]`, want: "abc123"},
		{name: "bare ID", photo: "abc123", want: "abc123"},
		{name: "empty array", photo: "[]"},
		{name: "malformed array", photo: "["},
		{name: "missing ID", photo: `[{"contentType":"image/jpeg"}]`},
		{name: "invalid ID", photo: `[{"id":"folder/image"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePhotoID(test.photo)
			if got != test.want || (err != nil) != (test.want == "") {
				t.Fatalf("parsePhotoID = (%q, %v), want ID %q", got, err, test.want)
			}
		})
	}
}
