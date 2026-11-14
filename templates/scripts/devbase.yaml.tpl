{{- /*
  scripts/devbase.yaml is the configuration file of the devbase CLI. Other
  modules add its top-level sections through the devbaseYaml module hook.
  Each value is a map from a section name to the section, and the template
  merges the values in hook order:

    {{ stencil.AddToModuleHook "github.com/getoutreach/devbase" "devbaseYaml" (list (dict "graphql" $lint)) }}

  Known sections: graphql (stencil-graphql) and release (stencil-base). The
  file renders only when a hook adds content. Until a module adds the
  release section, a hand-written one stays in the release block.
*/ -}}
{{- $sections := dict }}
{{- range stencil.GetModuleHook "devbaseYaml" }}
{{- if not (kindIs "map" .) }}
{{- fail (printf "module hook devbaseYaml: got a %s, want a map" (kindOf .)) }}
{{- end }}
{{- $sections = mustMergeOverwrite $sections . }}
{{- end }}
{{- if not $sections }}
{{- file.Skip "no module adds a section to scripts/devbase.yaml" }}
{{- end -}}
# Configuration for the devbase CLI. Stencil renders this file from the
# sections that other modules add. Change only the release block by hand.
{{ $sections | toYaml }}
{{- if not (hasKey $sections "release") }}
## <<Stencil::Block(release)>>
{{- with file.Block "release" }}
{{ . }}
{{- end }}
## <</Stencil::Block>>
{{- end }}
