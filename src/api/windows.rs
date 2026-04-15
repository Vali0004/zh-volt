use crate::api::StatusOutputType;
use crate::olt::olt_manager::SharedOltState;
use crate::olt::olt_manager::get_olts_vec;
use scanf::sscanf;
use serde_json::to_string;

use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::time::Duration;
use std::thread;

pub fn create_unix_listen(socket_path: &String) -> std::io::Result<TcpListener> {
	TcpListener::bind(socket_path)
}

pub fn process(listener: TcpListener, state: SharedOltState) {
	for stream in listener.incoming() {
		match stream {
			Err(err) => println!("Connection failed: {}", err),
			Ok(mut stream) => {
				let state_clone = state.clone();

				thread::spawn(move || {
					let mut count: i64 = 1;
					let mut buffer = [0; 1024];

					let _ = stream.set_read_timeout(Some(Duration::from_millis(600)));

					if let Ok(size) = stream.read(&mut buffer) {
						let mut data = std::str::from_utf8(&buffer[..size]).unwrap();

						if data.ends_with('\n') {
							data = data.trim_end_matches('\n');
						}

						if !buffer.is_empty() {
							if let Err(err) = sscanf!(data, "{}", &mut count) {
								println!("Error parsing count: {}", err);
							}
						}
					}

					while count == -1 || count > 0 {
						if count > 0 {
							count -= 1;
						}

						let data = match get_olts_vec(state_clone.clone()) {
							Ok(d) => d,
							Err(_) => return,
						};

						let mut data_string = match to_string(&data) {
							Ok(s) => s,
							Err(_) => return,
						};

						data_string.push('\n');

						if let Err(err) = stream.write_all(data_string.as_bytes()) {
							println!("Error writing to socket: {}", err);
							return;
						}

						if let Err(err) = stream.flush() {
							println!("Error flushing socket: {}", err);
						}

						thread::sleep(Duration::from_millis(300));
					}
				});
			}
		}
	}
}

pub fn client_status(
	conn: String,
	watch: bool,
	_output: StatusOutputType,
) -> Result<(), std::io::Error> {

	let mut stream = match TcpStream::connect(conn) {
		Err(err) => {
			println!("Error connecting to socket: {}", err);
			return Err(err);
		}
		Ok(stream) => stream,
	};

	if watch {
		let count: i64 = -1;
		let _ = stream.write(count.to_string().as_bytes());
	}

	loop {
		let mut buffer = [0; (1024 ^ 2) * 24];

		match stream.read(&mut buffer) {
			Err(err) => {
				println!("Error reading from socket: {}", err);
				return Err(err);
			}
			Ok(size) => {
				if size == 0 {
					break;
				}

				let data = std::str::from_utf8(&buffer[..size]).unwrap();
				print!("{}", data);
			}
		}

		if !watch {
			break;
		}
	}

	Ok(())
}