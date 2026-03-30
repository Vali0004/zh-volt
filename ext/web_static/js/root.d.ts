type olts = olt[]

type onu = {
	id: number
	status: string
	uptime: string
	sn: string
	voltage: number
	current: number
	tx_power: number
	rx_power: number
	temperature: number
}

type olt = {
	uptime: string
	mac_addr: string
	firmware_version: string
	olt_dna: string
	temperature: number
	max_temperature: number
	omci_mode: number
	omci_error: number
	online_onu: number
	max_onu: number
	onus: onu[]
}