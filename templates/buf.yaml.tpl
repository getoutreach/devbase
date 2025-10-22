version: v1
name: github.com/getoutreach/{{ .Config.Name }}
deps: []
build:
  excludes:
    # pkg and internal may have test protos
    - pkg
    - internal
    - api/clients/node_modules
    - api/clients/node/node_modules
    ## <<Stencil::Block(buf_exclude_paths)>>
    {{ file.Block "buf_exclude_paths" }}
    ## <</Stencil::Block>>
lint:
  use:
    - BASIC
    # - STANDARD

