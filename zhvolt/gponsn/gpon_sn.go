package gponsn

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// GPON SN [4byte for vendor in ASCI][4byte for device vendor]
//
// exp: [54504c47][89bdcfd8] ([TPLG    ][hardware id])
type GPON_SN [8]byte

func (sn GPON_SN) IsValid() bool {
	if bytes.Equal(sn[:], (&GPON_SN{})[:]) {
		return false
	}
	return binary.BigEndian.Uint32(sn[4:]) > 0 && sn.Vendor() != nil
}

func (sn GPON_SN) Vendor() *GPON_VENDOR {
	var ver GPON_VENDOR
	if ver.UnmarshallText(sn[:]) == nil {
		return &ver
	}
	return nil
}

func (sn GPON_SN) String() string {
	return fmt.Sprintf("%s%s", sn[:4], hex.EncodeToString(sn[4:]))
}

func (sn GPON_SN) MarshalText() ([]byte, error) {
	return []byte(sn.String()), nil
}

func (sn *GPON_SN) UnmarshalText(data []byte) error {
	if len(data) < 8 {
		return ErrInvalid
	}

	var vendor GPON_VENDOR
	if err := vendor.UnmarshallText(data[:4]); err == nil {
		copy(sn[:], data)
		return nil
	}

	data, err := vendor.Cut(data)
	if err != nil {
		return err
	}
	copy(sn[4:], data)
	return nil
}
