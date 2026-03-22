pub mod api;
pub mod olt;
pub mod sn;

use clap::{Command, arg};

use crate::api::{Server, route, unix};
use crate::olt::olt_maneger::{OltManager, new_pcap_dev, new_share};

fn main() {
	let cmd = Command::new("zh-volt")
		.arg(arg!(-i --netdev <String> "Net device to watch packets").default_value("eth0"))
		.arg(arg!(-l --listen <String> "HTTP api or socket to listen").default_value("0.0.0.0:8081"))
		.get_matches();

	let net_dev = cmd.get_one::<String>("netdev").unwrap().to_string();
	let listen = cmd.get_one::<String>("listen").unwrap().to_string();
	let shared_olts = new_share();

	let dev = match new_pcap_dev(net_dev) {
		Err(err) => panic!("Error starting Pcap: {:?}", err),
		Ok(dev) => dev,
	};
	let mut manager = OltManager::new(dev, shared_olts.clone());
	let man = std::thread::spawn(move || manager.run());

	// Check if listen unix socket or http server
	if listen.starts_with("unix:") || listen.ends_with(".sock") {
		let listen = listen.replace("unix:", "");
		std::thread::spawn(
			move || match unix::create_unix_listen(&listen, shared_olts.clone()) {
				Ok(_) => (),
				Err(err) => panic!("Error creating unix socket: {}", err),
			},
		);
	} else {
		let server = Server::http(listen).unwrap();
		std::thread::spawn(move || route::create_router(server, shared_olts.clone()));
	}

	let _ = man.join().unwrap();
}
