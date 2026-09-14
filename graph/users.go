package graph

import (
	"encoding/json"
)

// PanelCreds — учётные данные VPN-пользователя, вшиваемые в entry-inbound графа.
type PanelCreds struct {
	Name     string
	UUID     string
	Password string
	Flow     string
	Remark   string
}

// InjectUsersIntoState добавляет креды пользователя в users каждого entry-inbound,
// если их там ещё нет. Вхождение считается существующим по uuid (уникален),
// для password-протоколов — по password.
func InjectUsersIntoState(st State, c PanelCreds) State {
	for i := range st.Nodes {
		if st.Nodes[i].Kind != KindInbound || !st.Nodes[i].Entry {
			continue
		}
		in, err := ParseInboundSettings(st.Nodes[i].Settings)
		if err != nil {
			continue
		}
		if hasUser(in.Users, c) {
			continue
		}
		in.Users = append(in.Users, InboundUser{Name: c.Name, UUID: c.UUID, Password: c.Password, Flow: c.Flow})
		raw, err := json.Marshal(in)
		if err != nil {
			continue
		}
		st.Nodes[i].Settings = raw
	}
	return st
}

// RemoveUserFromState убирает креды пользователя из всех entry-inbound графа.
func RemoveUserFromState(st State, c PanelCreds) State {
	for i := range st.Nodes {
		if st.Nodes[i].Kind != KindInbound || !st.Nodes[i].Entry {
			continue
		}
		in, err := ParseInboundSettings(st.Nodes[i].Settings)
		if err != nil {
			continue
		}
		out := in.Users[:0]
		for _, u := range in.Users {
			if matchesUser(u, c) {
				continue
			}
			out = append(out, u)
		}
		if len(out) == len(in.Users) {
			continue
		}
		in.Users = out
		raw, err := json.Marshal(in)
		if err != nil {
			continue
		}
		st.Nodes[i].Settings = raw
	}
	return st
}

func hasUser(users []InboundUser, c PanelCreds) bool {
	for _, u := range users {
		if matchesUser(u, c) {
			return true
		}
	}
	return false
}

func matchesUser(u InboundUser, c PanelCreds) bool {
	if c.UUID != "" && u.UUID != "" {
		return u.UUID == c.UUID
	}
	if c.Password != "" && u.Password != "" {
		return u.Password == c.Password
	}
	return u.Name == c.Name
}
