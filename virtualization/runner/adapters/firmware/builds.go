package firmware

type ovmfBuild struct {
	code, vars string
	format     string
	tpm        bool
}

var ovmfSearch = []ovmfBuild{
	{"/usr/share/edk2/ovmf/OVMF_CODE_4M.qcow2", "/usr/share/edk2/ovmf/OVMF_VARS_4M.qcow2", "qcow2", true},
	{"/usr/share/OVMF/OVMF_CODE_4M.fd", "/usr/share/OVMF/OVMF_VARS_4M.fd", "raw", true},
	{"/usr/share/edk2/x64/OVMF_CODE.4m.fd", "/usr/share/edk2/x64/OVMF_VARS.4m.fd", "raw", true},
	{"/usr/share/edk2/ovmf/OVMF_CODE.fd", "/usr/share/edk2/ovmf/OVMF_VARS.fd", "raw", false},
	{"/usr/share/OVMF/OVMF_CODE.fd", "/usr/share/OVMF/OVMF_VARS.fd", "raw", false},
	{"/usr/share/qemu/ovmf-x86_64-code.bin", "/usr/share/qemu/ovmf-x86_64-vars.bin", "raw", false},
}

var ovmfSecbootSearch = []ovmfBuild{
	{"/usr/share/edk2/ovmf/OVMF_CODE_4M.secboot.qcow2", "/usr/share/edk2/ovmf/OVMF_VARS_4M.secboot.qcow2", "qcow2", true},
	{"/usr/share/OVMF/OVMF_CODE_4M.secboot.fd", "/usr/share/OVMF/OVMF_VARS_4M.ms.fd", "raw", true},
	{"/usr/share/edk2/x64/OVMF_CODE.secboot.4m.fd", "/usr/share/edk2/x64/OVMF_VARS.4m.fd", "raw", true},
	{"/usr/share/edk2/ovmf/OVMF_CODE.secboot.fd", "/usr/share/edk2/ovmf/OVMF_VARS.secboot.fd", "raw", false},
	{"/usr/share/OVMF/OVMF_CODE.secboot.fd", "/usr/share/OVMF/OVMF_VARS.secboot.fd", "raw", false},
}
