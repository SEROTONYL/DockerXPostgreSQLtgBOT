package bot

import "testing"

func TestCommandParser_MemberCommandsRequirePrefix(t *testing.T) {
	p := NewCommandParser()

	okCases := []string{"!пленки", "! плёнки", ".пленки", ". плёнки", "!список", "! список", ".список", ". список"}
	for _, in := range okCases {
		cmd, _, ok := p.ParseCommand(in, false)
		if !ok {
			t.Fatalf("expected command parsed: %q", in)
		}
		if cmd != "пленки" && cmd != "список" {
			t.Fatalf("unexpected command %q from %q", cmd, in)
		}
	}

	badCases := []string{"пленки", "список", " /login"}
	for _, in := range badCases {
		if _, _, ok := p.ParseCommand(in, false); ok {
			t.Fatalf("command must not parse without allowed prefix: %q", in)
		}
	}

	if cmd, _, ok := p.ParseCommand("/login", true); !ok || cmd != "login" {
		t.Fatalf("unexpected admin slash parse: ok=%v cmd=%q", ok, cmd)
	}
}
