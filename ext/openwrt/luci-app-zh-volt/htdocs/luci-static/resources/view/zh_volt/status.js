'use strict';
'require view';
'require dom';
'require poll';
'require rpc';

var callVoltStatus = rpc.declare({
	object: 'luci.zh_volt',
	method: 'status',
	expect: { data: [] }
});

var callVoltOnuActivate = rpc.declare({
	object: 'luci.zh_volt',
	method: 'onu_activate',
	expect: { result: false }
});

var callVoltOnuDeactivate = rpc.declare({
	object: 'luci.zh_volt',
	method: 'onu_deactivate',
	expect: { result: false }
});

// Format same for C printf %d, %s...
function format(msg, ...args) {
	var i = 0;
	return msg.replace(/%[sdf]/g, function (match) {
		return typeof args[i] !== 'undefined' ? args[i++] : match;
	});
}

return view.extend({
	handleSaveApply: null,
	handleSave: null,
	handleReset: null,

	render: function () {
		var container = E('div', { 'class': 'cbi-map' }, [
			E('h2', { 'name': 'content' }, _('OLT Status (zh-volt)')),
			E('div', { 'class': 'cbi-map-descr' }, _('Overview of the status of the OLT and ONUs connected to the network.'))
		]);

		var oltsContainer = E('div', { 'id': 'olts-container' }, [
			E('div', { 'class': 'spinning' }, _('Loading data from OLT...'))
		]);

		container.appendChild(oltsContainer);
		poll.add(async function () {
			try {
				const data = await callVoltStatus();
				var contentNode = document.getElementById('olts-container');

				if (!contentNode) return;
				dom.content(contentNode, '');
				if (!data || data.length === 0) {
					contentNode.appendChild(E('div', { 'class': 'alert-message warning' }, _('No OLT discovered yet.')));
					return;
				}

				data.forEach(function (olt) {
					var oltTable = E('table', { 'class': 'table' }, [
						// E('tr', { 'class': 'tr' }, [
						// 	E('td', { 'class': 'td left', 'style': 'width:33%' }, E('strong', _('OLT MAC:'))),
						// 	E('td', { 'class': 'td left' }, olt.mac_addr)
						// ]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td left' }, E('strong', _('Uptime') + ":")),
							E('td', { 'class': 'td left' }, olt.uptime)
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td left' }, E('strong', _('Firmware Version') + ":")),
							E('td', { 'class': 'td left' }, olt.firmware_version)
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td left' }, E('strong', _('DNA OLT') + ":")),
							E('td', { 'class': 'td left' }, olt.olt_dna)
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td left' }, E('strong', _('Temperature') + ":")),
							E('td', { 'class': 'td left' }, format(_('%s °C (Max: %s °C)'), String(olt.temperature), String(olt.max_temperature)))
						]),
						E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td left' }, E('strong', 'ONUs:')),
							E('td', { 'class': 'td left' }, format(_('Online: %d / Max: %d'), String(olt.online_onu), String(olt.max_onu)))
						])
					]);

					var onuRows = [
						E('tr', { 'class': 'tr table-titles' }, [
							E('th', { 'class': 'th' }, _('ID')),
							E('th', { 'class': 'th' }, _('Status')),
							E('th', { 'class': 'th' }, _('GPON Serial Number (SN)')),
							E('th', { 'class': 'th' }, _('Uptime')),
							E('th', { 'class': 'th' }, _('Temperature (°C)')),
							E('th', { 'class': 'th' }, _('RX/TX Power')),
							E('th', { 'class': 'th' }, _('Actions')),
						])
					];

					olt.onus.forEach(function (onu) {
						var statusStyle = '';
						var statusText = onu.status[0].toUpperCase() + onu.status.slice(1);

						if (onu.status === 'online') {
							statusStyle = 'color: #4CAF50; font-weight: bold;';
						} else if (onu.status === 'offline') {
							statusStyle = 'color: #9E9E9E;';
						} else if (onu.status === 'disconnected') {
							statusStyle = 'color: #F44336; font-weight: bold;';
						} else if (onu.status === 'omci') {
							statusStyle = 'color: #FF9800; font-weight: bold;';
						} else {
							statusStyle = 'color: #9E9E9E;';
						}

						var enableBtn = E('button', {
							'class': 'cbi-button cbi-button-action',
							'click': function (ev) {
								var btn = ev.target;
								var originalText = btn.textContent;

								btn.disabled = true;
								btn.textContent = _('Wait...');
								btn.classList.add('spinning');
								ui.add_message('info', format(_('Activating ONU %d on OLT %s...'), onu.id, olt.mac_addr));

								callVoltOnuActivate(olt.mac_addr, onu.id).then(function (res) {
									btn.disabled = false;
									btn.textContent = originalText;
									btn.classList.remove('spinning');

									if (res.result === true) {
										ui.add_message('success', format(_('ONU %d activated successfully on OLT %s.'), onu.id, olt.mac_addr));
										poll.check();
									} else {
										ui.add_message('danger', format(_('Failed to activate ONU %d on OLT %s.'), onu.id, olt.mac_addr));
									}
								}).catch(function (err) {
									btn.disabled = false;
									btn.textContent = originalText;
									btn.classList.remove('spinning');
									ui.add_message('danger', format(_('Error making RPC request to activate ONU %d on OLT %s: %s'), onu.id, olt.mac_addr, err.message));
								});
							}
						}, _('Enable'));

						var disableBtn = E('button', {
							'class': 'cbi-button cbi-button-reset',
							'click': function (ev) {
								var btn = ev.target;
								var originalText = btn.textContent;

								btn.disabled = true;
								btn.textContent = _('Wait...');
								btn.classList.add('spinning');

								ui.add_message('info', format(_('Deactivating ONU %d on OLT %s...'), onu.id, olt.mac_addr));

								callVoltOnuDeactivate(olt.mac_addr, onu.id).then(function (res) {
									btn.disabled = false;
									btn.textContent = originalText;
									btn.classList.remove('spinning');

									if (res.result === true) {
										ui.add_message('success', format(_('ONU %d deactivated successfully on OLT %s.'), onu.id, olt.mac_addr));
										poll.check();
									} else {
										ui.add_message('danger', format(_('Failed to deactivate ONU %d on OLT %s.'), onu.id, olt.mac_addr));
									}
								}).catch(function (err) {
									btn.disabled = false;
									btn.textContent = originalText;
									btn.classList.remove('spinning');
									ui.add_message('danger', format(_('Error making RPC request to deactivate ONU %d on OLT %s: %s'), onu.id, olt.mac_addr, err.message));
								});
							}
						}, _('Disable'));

						onuRows.push(E('tr', { 'class': 'tr' }, [
							E('td', { 'class': 'td', 'data-title': _('ID') }, String(onu.id)),
							E('td', { 'class': 'td', 'data-title': _('Status'), 'style': statusStyle }, statusText),
							E('td', { 'class': 'td', 'data-title': _('GPON SN') }, onu.sn === '0000-00000000' ? '-' : onu.sn),
							E('td', { 'class': 'td', 'data-title': _('Uptime') }, onu.uptime === '0s' ? '-' : onu.uptime),
							E('td', { 'class': 'td', 'data-title': _('Temperature') }, onu.temperature === 0 ? '-' : String(onu.temperature)),
							E('td', { 'class': 'td', 'data-title': _('RX/TX Power') },
								onu.status === 'online' ? format(_("%d / %d"), String(onu.rx_power), String(onu.tx_power)) : '-'
							),
							E('td', { 'class': 'td', 'data-title': _('Actions') }, [
								enableBtn,
								E('span', { 'style': 'margin-right: 5px;' }),
								disableBtn
							])
						]));
					});

					var onuTable = E('table', { 'class': 'table w100' }, onuRows);
					var oltSection = E('fieldset', { 'class': 'cbi-section' }, [
						E('h3', _('OLT information') + ": " + olt.mac_addr),
						E('div', { 'class': 'cbi-section-node' }, oltTable),

						E('br'),

						E('h3', 'Status das ONUs'),
						E('div', { 'class': 'cbi-section-node' }, onuTable)
					]);

					contentNode.appendChild(oltSection);
				});

			} catch (err) {
				var contentNode = document.getElementById('olts-container');
				if (contentNode) {
					dom.content(contentNode, E('div', { 'class': 'alert-message warning' }, _('Fail to get data from zh-volt daemon. Is the service running?' + String(err))));
				}
			}
		}, 5);

		return container;
	}
});