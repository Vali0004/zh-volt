// Main Javascript file

/**
 * Format a string with placeholders
 * @param {string} msg
 * @param {...any} args
 * @returns {string}
 */
function format(msg, ...args) {
	var i = 0;
	return msg.replace(/%[sdf]/g, function (match) {
		return typeof args[i] !== 'undefined' ? args[i++] : match;
	});
}

async function fetch_data() {
	try {
		/** @type {olts} */
		const data = await fetch("/data.json").then((res) => res.json(), err => err);
		for (const olt_data of data) {
			olt(olt_data);
		}
	} catch (err) {
		console.error(err);
	}
}

/**
 * Update ONU information in the UI
 * @param {Element} root_element
 * @param {onu} data
 */
function format_onu(root_element, data) {
	/*
	<div class="onu_info" status="{{.Status}}">
		<div class="onu_actions">
			<span class="onu_id">ID: {{ .ID }}</span>
			<button>{{ if ne .Status 0 }}Disable{{else}}Enable{{end}}</button>
		</div>
		<div class="onu_data">
			<div>
				<span>Status: {{ .Status.String }}</span>
			</div>
			{{ if ne .Status 0 }}
			<div>
				<span>GPON SN: {{ .SN }}</span>
			</div>
			{{end}}
			{{ if eq .Status 1 }}
			<div>
				<span>Uptime: {{ .Uptime }}</span>
			</div>
			<div>
				<span>Temperature: {{ .Temperature }} °C</span>
			</div>
			<div>
				<span>Current: {{.Current}} mA</span>
			</div>
			<div>
				<span>TX Power: {{.TxPower}} dBm</span>
			</div>
			<div>
				<span>RX Power: {{.RxPower}} dBm</span>
			</div>
			{{end}}
		</div>
	</div>
	*/
	let onu = root_element.querySelector(`.onu_info[onu-id="${data.id}"]`);
	if (!onu) {
		onu = document.createElement("div");
		onu.classList.add("onu_info");
		onu.setAttribute("onu-id", data.id.toString());
		root_element.appendChild(onu);
	}
	onu.setAttribute("status", data.status.toString());

	let actions_div = onu.querySelector(".onu_actions");
	if (!actions_div) {
		actions_div = document.createElement("div");
		actions_div.classList.add("onu_actions");
		onu.appendChild(actions_div);
	}

	let id_span = actions_div.querySelector(".onu_id");
	if (!id_span) {
		id_span = document.createElement("span");
		id_span.classList.add("onu_id");
		actions_div.appendChild(id_span);
	}
	id_span.textContent = format("ID: %d", data.id);

	let action_button = actions_div.querySelector("button");
	if (!action_button) {
		action_button = document.createElement("button");
		actions_div.appendChild(action_button);
	}
	action_button.textContent = data.status === "online" ? "Disable" : "Enable";
	action_button.onclick = async function () { }

	let data_div = onu.querySelector(".onu_data");
	if (!data_div) {
		data_div = document.createElement("div");
		data_div.classList.add("onu_data");
		onu.appendChild(data_div);
	}

	let status_div = data_div.querySelector(".onu_status");
	if (!status_div) {
		status_div = document.createElement("div");
		status_div.classList.add("onu_status");
		data_div.appendChild(status_div);
	}
	status_div.textContent = format("Status: %s", data.status);

	if (data.status === "online" || data.status === "omci") {
		let sn_div = data_div.querySelector(".onu_sn");
		if (!sn_div) {
			sn_div = document.createElement("div");
			sn_div.classList.add("onu_sn");
			data_div.appendChild(sn_div);
		}
		sn_div.textContent = format("GPON SN: %s", data.sn.toUpperCase());
	}

	let uptime_div = data_div.querySelector(".onu_uptime");
	if (!uptime_div) {
		uptime_div = document.createElement("div");
		uptime_div.classList.add("onu_uptime");
		data_div.appendChild(uptime_div);
	}
	uptime_div.textContent = format("Uptime: %s", data.uptime);

	let temp_div = data_div.querySelector(".onu_temperature");
	if (!temp_div) {
		temp_div = document.createElement("div");
		temp_div.classList.add("onu_temperature");
		data_div.appendChild(temp_div);
	}
	temp_div.textContent = format("Temperature: %d °C", data.temperature);

	let current_div = data_div.querySelector(".onu_current");
	if (!current_div) {
		current_div = document.createElement("div");
		current_div.classList.add("onu_current");
		data_div.appendChild(current_div);
	}
	current_div.textContent = format("Current: %d mA", data.current);

	let tx_power_div = data_div.querySelector(".onu_tx_power");
	if (!tx_power_div) {
		tx_power_div = document.createElement("div");
		tx_power_div.classList.add("onu_tx_power");
		data_div.appendChild(tx_power_div);
	}
	tx_power_div.textContent = format("TX Power: %d dBm", data.tx_power);

	let rx_power_div = data_div.querySelector(".onu_rx_power");
	if (!rx_power_div) {
		rx_power_div = document.createElement("div");
		rx_power_div.classList.add("onu_rx_power");
		data_div.appendChild(rx_power_div);
	}
	rx_power_div.textContent = format("RX Power: %d dBm", data.rx_power);
}

/**
 * Process OLT data
 * @param {olt} data
 */
function olt(data) {
	const id = `olt-${data.mac_addr}`;
	let root = document.getElementById(id);
	if (!root) {
		root = document.createElement("div");
		root.id = id;
		root.classList.add("olt");
		document.body.appendChild(root);
	}

	let info_div = document.getElementById(`${id}-info`);
	if (!info_div) {
		info_div = document.createElement("div");
		info_div.id = `${id}-info`;
		info_div.classList.add("olt-info");
		root.appendChild(info_div);
	}

	let info_h2 = info_div.querySelector("h2");
	if (!info_h2) {
		info_h2 = document.createElement("h2");
		info_div.appendChild(info_h2);
	}
	// OLT: {{ .Mac }} (Version {{ .FirmwareVersion }}, OLT DNA: {{ .DNA }})
	info_h2.textContent = format("OLT: %s (Version %s, OLT DNA: %s)", data.mac_addr, data.firmware_version, data.olt_dna);

	let info_body = root.querySelector(".olt_info");
	if (!info_body) {
		info_body = document.createElement("div");
		info_body.classList.add("olt_info");
		root.appendChild(info_body);
	}

	let info_ul = info_body.querySelector("ul");
	if (!info_ul) {
		info_ul = document.createElement("ul");
		info_body.appendChild(info_ul);
	}

	/*
	<li><span class="olt-info-max-onu">Max ONU: {{ .MaxONU }}, Online ONUs {{ .OnlineONU}}</span></li>
	<li><span class="olt-info-omci">OMCI Mode: {{ .OMCIMode }}, OMCI Errors {{ .OMCIErr }}</span></li>
	<li><span class="olt-info-temperature">Current Temperature: {{ .Temperature }}°C</span></li>
	<li><span class="olt-info-max-temperature">Max Temperature: {{ .MaxTemperature }}°C</span></li>
	<li><span class="olt-info-uptime">Uptime: {{ .Uptime }}</span></li>
	*/
	let max_onu_li = info_ul.querySelector(".olt-info-max-onu");
	if (!max_onu_li) {
		max_onu_li = document.createElement("li");
		max_onu_li.classList.add("olt-info-max-onu");
		info_ul.appendChild(max_onu_li);
	}
	max_onu_li.textContent = format("Max ONU: %d, Online ONUs: %d", data.max_onu, data.online_onu);

	let omci_li = info_ul.querySelector(".olt-info-omci");
	if (!omci_li) {
		omci_li = document.createElement("li");
		omci_li.classList.add("olt-info-omci");
		info_ul.appendChild(omci_li);
	}
	omci_li.textContent = format("OMCI Mode: %s, OMCI Errors: %d", data.omci_mode, data.omci_error);

	let temp_li = info_ul.querySelector(".olt-info-temperature");
	if (!temp_li) {
		temp_li = document.createElement("li");
		temp_li.classList.add("olt-info-temperature");
		info_ul.appendChild(temp_li);
	}
	temp_li.textContent = format("Current Temperature: %d°C", data.temperature);

	let max_temp_li = info_ul.querySelector(".olt-info-max-temperature");
	if (!max_temp_li) {
		max_temp_li = document.createElement("li");
		max_temp_li.classList.add("olt-info-max-temperature");
		info_ul.appendChild(max_temp_li);
	}
	max_temp_li.textContent = format("Max Temperature: %d°C", data.max_temperature);

	let uptime_li = info_ul.querySelector(".olt-info-uptime");
	if (!uptime_li) {
		uptime_li = document.createElement("li");
		uptime_li.classList.add("olt-info-uptime");
		info_ul.appendChild(uptime_li);
	}
	uptime_li.textContent = format("Uptime: %s", data.uptime);

	// ONUs
	let onu_div = root.querySelector(".onu");
	if (!onu_div) {
		onu_div = document.createElement("div");
		onu_div.classList.add("onu");
		root.appendChild(onu_div);
	}

	for (const onu_data of data.onus)
		format_onu(onu_div, onu_data);
}

/**
 * Wait for a specified amount of time
 * @param {number} ms
 * @returns
 */
async function wait(ms) {
	let timeout;
	try {
		return await new Promise((resolve) => timeout = setTimeout(resolve, ms));
	} finally {
		return clearTimeout(timeout);
	}
}

async function main() {
	while (true) {
		await fetch_data();

		await wait(1000);
	}
}

const old_onload = window.onload;
window.onload = function () {
	if (old_onload) old_onload.call(this, ...arguments);
	main();
}