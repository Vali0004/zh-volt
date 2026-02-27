package packet

import "encoding/binary"

func Convert(request, reqType uint16, reserve, check0, check1 uint8, data []byte) (pkt Packet, err error) {
	newData := make([]byte, len(OltMagic), 50)
	copy(newData, OltMagic)
	
	newData = binary.BigEndian.AppendUint16(newData, request)
	newData = binary.BigEndian.AppendUint16(newData, reqType)
	newData = append(newData, reserve, check0, check1)
	if data != nil {
		copy(newData[len(newData):cap(newData)], data)
	}

	err = pkt.UnmarshalBinary(newData[:cap(newData)])
	return
}
