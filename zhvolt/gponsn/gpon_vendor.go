package gponsn

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNullSn     = errors.New("null sn")
	ErrInvalid    = errors.New("invalid GPON SN")
	ErrVendorName = errors.New("invalid vendor name")
)

// GPON Vendor IDs, from https://hack-gpon.org/vendor/
type GPON_VENDOR uint64

const (
	VENDOR_ADTN = GPON_VENDOR(0x4144544e)
	VENDOR_ALCL = GPON_VENDOR(0x414c434c)
	VENDOR_ALLG = GPON_VENDOR(0x414c4c47)
	VENDOR_AVMG = GPON_VENDOR(0x41564d47)
	VENDOR_ASKY = GPON_VENDOR(0x41534b59)
	VENDOR_CDKT = GPON_VENDOR(0x43444B54)
	VENDOR_CIGG = GPON_VENDOR(0x43494747)
	VENDOR_CXNK = GPON_VENDOR(0x43584e4b)
	VENDOR_DDKT = GPON_VENDOR(0x44444b54)
	VENDOR_DLNK = GPON_VENDOR(0x444c4e4b)
	VENDOR_DSNW = GPON_VENDOR(0x44534e57)
	VENDOR_ELTX = GPON_VENDOR(0x454c5458)
	VENDOR_FHTT = GPON_VENDOR(0x46485454)
	VENDOR_GMTK = GPON_VENDOR(0x474d544b)
	VENDOR_GNXS = GPON_VENDOR(0x474e5853)
	VENDOR_GPNC = GPON_VENDOR(0x47504E43)
	VENDOR_GPON = GPON_VENDOR(0x47504f4e)
	VENDOR_GTHG = GPON_VENDOR(0x47544847)
	VENDOR_HALN = GPON_VENDOR(0x48414c4e)
	VENDOR_HBMT = GPON_VENDOR(0x48424d54)
	VENDOR_HUMA = GPON_VENDOR(0x48554d41)
	VENDOR_HWTC = GPON_VENDOR(0x48575443)
	VENDOR_ICTR = GPON_VENDOR(0x49435452)
	VENDOR_ISKT = GPON_VENDOR(0x49534b54)
	VENDOR_KAON = GPON_VENDOR(0x4b414f4e)
	VENDOR_LEOX = GPON_VENDOR(0x4c454f58)
	VENDOR_LQDE = GPON_VENDOR(0x4c514445)
	VENDOR_NOKG = GPON_VENDOR(0x4e4f4b47)
	VENDOR_NOKW = GPON_VENDOR(0x4e4f4b57)
	VENDOR_MSTC = GPON_VENDOR(0x4d535443)
	VENDOR_PTIN = GPON_VENDOR(0x5054494e)
	VENDOR_RTKG = GPON_VENDOR(0x52544b47)
	VENDOR_SCOM = GPON_VENDOR(0x53434f4d)
	VENDOR_SKYW = GPON_VENDOR(0x534b5957)
	VENDOR_SMBS = GPON_VENDOR(0x534d4253)
	VENDOR_SPGA = GPON_VENDOR(0x53504741)
	VENDOR_TMBB = GPON_VENDOR(0x544d4242)
	VENDOR_TPLG = GPON_VENDOR(0x54504c47)
	VENDOR_UBNT = GPON_VENDOR(0x55424e54)
	VENDOR_UGRD = GPON_VENDOR(0x55475244)
	VENDOR_YHTC = GPON_VENDOR(0x59485443)
	VENDOR_ZNTS = GPON_VENDOR(0x5a4e5453)
	VENDOR_ZRMT = GPON_VENDOR(0x5a524d54)
	VENDOR_ZTEG = GPON_VENDOR(0x5a544547)
	VENDOR_ZYWN = GPON_VENDOR(0x5a59574e)
	VENDOR_ZYXE = GPON_VENDOR(0x5a595845)
)

var vendorNames = map[GPON_VENDOR]string{
	VENDOR_ADTN: "Adtran",
	VENDOR_ALCL: "Nokia/Alcatel-Lucent",
	VENDOR_ALLG: "ALLNET",
	VENDOR_AVMG: "AVM (FRITZ!Box)",
	VENDOR_ASKY: "Askey",
	VENDOR_CDKT: "KingType",
	VENDOR_CIGG: "Cig",
	VENDOR_CXNK: "Calix",
	VENDOR_DDKT: "DKT",
	VENDOR_DLNK: "Dlink",
	VENDOR_DSNW: "DASAN",
	VENDOR_ELTX: "Eltex",
	VENDOR_FHTT: "FiberHome",
	VENDOR_GMTK: "GemTek",
	VENDOR_GNXS: "Genexis",
	VENDOR_GPNC: "NuCom",
	VENDOR_GPON: "Generic vendor name",
	VENDOR_GTHG: "Alcatel-Lucent (ODM)",
	VENDOR_HALN: "HALNy",
	VENDOR_HBMT: "HiSense",
	VENDOR_HUMA: "Humax",
	VENDOR_HWTC: "Huawei",
	VENDOR_ICTR: "Icotera",
	VENDOR_ISKT: "Iskratel",
	VENDOR_KAON: "KAONMEDIA",
	VENDOR_LEOX: "LEOX",
	VENDOR_LQDE: "Lantiq",
	VENDOR_NOKG: "Nokia (GemTek ODM)",
	VENDOR_NOKW: "Nokia (GemTek ODM)",
	VENDOR_MSTC: "Mitrastar",
	VENDOR_PTIN: "Altice/PT Inovação",
	VENDOR_RTKG: "Realtek",
	VENDOR_SCOM: "Sercomm",
	VENDOR_SKYW: "Skyworth",
	VENDOR_SMBS: "Sagemcom",
	VENDOR_SPGA: "SourcePhotonics",
	VENDOR_TMBB: "Technicolor",
	VENDOR_TPLG: "TP-Link",
	VENDOR_UBNT: "Ubiquiti",
	VENDOR_UGRD: "UGrid",
	VENDOR_YHTC: "Youhua",
	VENDOR_ZNTS: "DZS",
	VENDOR_ZRMT: "Zaram",
	VENDOR_ZTEG: "ZTE",
	VENDOR_ZYWN: "Zyxel",
	VENDOR_ZYXE: "Zyxel",
}

func (vendor *GPON_VENDOR) Cut(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("needs 4byte or more")
	}

	ver := GPON_VENDOR(binary.BigEndian.Uint32(data[:4]))
	if _, ok := vendorNames[ver]; ok {
		*vendor = ver
		return data[4:], nil
	}

	for ver, name := range vendorNames {
		if strings.Contains(string(data), name) {
			*vendor = ver
			return data[len(name):], nil
		}
	}

	return data, ErrVendorName
}

func (vendor *GPON_VENDOR) UnmarshallText(data []byte) error {
	if hex, err := hex.DecodeString(string(data)); err == nil {
		_, err := vendor.Cut(hex)
		return err
	}
	_, err := vendor.Cut(data)
	return err
}

func (vendor GPON_VENDOR) MarshalText() ([]byte, error) {
	if vendorName, ok := vendorNames[vendor]; ok {
		return []byte(vendorName), nil
	}
	return []byte("Unknown vendor"), nil
}
