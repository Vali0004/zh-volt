'use strict';
'require view';
'require form';

return view.extend({
	render: function () {
		var m, s, o;

		m = new form.Map('zh-volt', _('ZH-Volt Configuration'),
			_('Configure network interface and socket options for the daemon.'));

		s = m.section(form.NamedSection, 'main', 'zh-volt', _('Main Settings'));
		s.addremove = false;

		o = s.option(form.Value, 'netdev', _('Network Interface'),
			_('The physical or logical interface where the OLT is connected (e.g. eth0, sfp).'));
		o.rmempty = false;

		o = s.option(form.Value, 'listen', _('Unix Socket / Listening Address'),
			_('UNIX socket path or HTTP address (e.g. /var/run/zh-volt.sock or 0.0.0.0:8081).'));
		o.rmempty = false;
		o.placeholder = '/var/run/zh-volt.sock';

		return m.render();
	}
});