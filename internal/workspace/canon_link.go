package workspace

import (
	"fmt"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func UpsertCanonLink(links []model.WorkspaceCanonLink, alias, role string) (updated []model.WorkspaceCanonLink, replacedAlias string, idempotent bool) {
	alias = strings.TrimSpace(alias)
	role = strings.TrimSpace(role)
	updated = append([]model.WorkspaceCanonLink(nil), links...)
	for i, link := range updated {
		if !strings.EqualFold(strings.TrimSpace(link.Role), role) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(link.Alias), alias) {
			return updated, "", true
		}
		replacedAlias = strings.TrimSpace(link.Alias)
		updated[i].Alias = alias
		updated[i].Role = role
		return updated, replacedAlias, false
	}
	updated = append(updated, model.WorkspaceCanonLink{Alias: alias, Role: role})
	return updated, "", false
}

func RegistrationHasCanonLink(registration model.WorkspaceRegistration, alias, role string) bool {
	alias = strings.TrimSpace(alias)
	role = strings.TrimSpace(role)
	if alias == "" {
		return false
	}
	for _, link := range registration.CanonLinks {
		if !strings.EqualFold(strings.TrimSpace(link.Alias), alias) {
			continue
		}
		if role == "" || strings.EqualFold(strings.TrimSpace(link.Role), role) {
			return true
		}
	}
	return false
}

func FilterCanonLinksByRole(links []model.WorkspaceCanonLink, role string) []model.WorkspaceCanonLink {
	role = strings.TrimSpace(role)
	if role == "" {
		return append([]model.WorkspaceCanonLink(nil), links...)
	}
	filtered := make([]model.WorkspaceCanonLink, 0, len(links))
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.Role), role) {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

func FormatCanonLinkAvailability(links []model.WorkspaceCanonLink) string {
	if len(links) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, fmt.Sprintf("id=%q role=%q", strings.TrimSpace(link.Alias), strings.TrimSpace(link.Role)))
	}
	return strings.Join(parts, ", ")
}

func FindRegisteredWorkspace(registry model.RegistryFile, alias string) (string, model.WorkspaceRegistration, bool) {
	alias = strings.TrimSpace(alias)
	if alias == "" || registry.Workspaces == nil {
		return "", model.WorkspaceRegistration{}, false
	}
	if registration, ok := registry.Workspaces[alias]; ok {
		registration.Name = alias
		return alias, registration, true
	}
	for name, registration := range registry.Workspaces {
		if strings.EqualFold(name, alias) {
			registration.Name = name
			return name, registration, true
		}
	}
	return "", model.WorkspaceRegistration{}, false
}
