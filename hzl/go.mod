module github.com/crispuscrew/hyprzinc/hzl

go 1.24.2

toolchain go1.24.13

// Shared functional core — hzl launches apps through the same code path as hzc
// (core/launch). Local replace + vendor keeps the hermetic per-module build.
require github.com/crispuscrew/hyprzinc/core v0.0.0

require github.com/BurntSushi/toml v1.5.0 // indirect

replace github.com/crispuscrew/hyprzinc/core => ../core
