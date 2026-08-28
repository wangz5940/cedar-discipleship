package user

import "testing"

func TestMemberNamePinyin(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "common chinese name", in: "张佳乐", want: "zhangjiale"},
		{name: "mixed ascii and chinese", in: "A 王明 01", want: "awangming01"},
		{name: "unknown han fallback", in: "龘", want: "u9f98"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memberNamePinyin(tt.in); got != tt.want {
				t.Fatalf("memberNamePinyin(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
