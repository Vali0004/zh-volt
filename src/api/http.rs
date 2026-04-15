use std::{path::PathBuf, str::FromStr};

use crate::olt::olt_manager::{SharedOltState, get_olts_vec};
use rust_embed::Embed;
use serde_json::to_string_pretty;
use tiny_http::{Header, Response, Server};

#[derive(Embed)]
#[folder = "./ext/web_static/"]
struct Asset;

pub fn process(server: Server, state: SharedOltState) {
	std::thread::spawn(move || {
		for req in server.incoming_requests() {
			let header_cors = Header::from_str("Access-Control-Allow-Origin: *").unwrap();
			let res_html = Header::from_str("Content-Type: text/html; utf-8").unwrap();
			let res_css = Header::from_str("Content-Type: text/css; utf-8").unwrap();
			let res_js = Header::from_str("Content-Type: application/javascript; utf-8").unwrap();
			let res_json = Header::from_str("Content-Type: application/json; utf-8").unwrap();

			match (req.method(), req.url()) {
				(tiny_http::Method::Get, "/" | "/index" | "/index.html" | "/index.htm") => {
					let data = Asset::get("index.html").unwrap().data.to_vec();
					let res = Response::from_data(data);
					req
						.respond(res.with_header(res_html).with_header(header_cors))
						.unwrap();
				}
				(tiny_http::Method::Get, "/data" | "/data.json") => {
					let data = match get_olts_vec(state.clone()) {
						Err(_) => {
							req
								.respond(
									Response::from_string("Error getting OLTs")
										.with_status_code(500)
										.with_header(res_json)
										.with_header(header_cors),
								)
								.unwrap();
							return;
						}
						Ok(data) => data,
					};

					match to_string_pretty(&data) {
						Err(_) => {
							req
								.respond(
									Response::from_string("Error serializing JSON")
										.with_status_code(500)
										.with_header(res_json)
										.with_header(header_cors),
								)
								.unwrap();
						}
						Ok(pretty_json) => {
							req
								.respond(
									Response::from_string(pretty_json)
										.with_status_code(200)
										.with_header(res_json)
										.with_header(header_cors),
								)
								.unwrap();
						}
					};
				}
				_ => {
					let url = req.url();
					if url.starts_with("/static/") {
						let path = PathBuf::from(&url[8..]);
						let path_ext = path.clone();
						match Asset::get(path.to_str().unwrap()) {
							Some(file) => {
								let res = Response::from_data(file.data);
								if let Some(ext) = path_ext.extension() {
									if ext == "js" || ext == "mjs" || ext == "cjs" {
										req
											.respond(res.with_header(res_js).with_header(header_cors))
											.unwrap();
										continue;
									} else if ext == "css" {
										req
											.respond(res.with_header(res_css).with_header(header_cors))
											.unwrap();
										continue;
									}
								}
								req.respond(res.with_header(header_cors)).unwrap();
							}
							None => {
								req
									.respond(
										Response::from_string("Error opening file")
											.with_status_code(404)
											.with_header(header_cors),
									)
									.unwrap();
							}
						}
					} else {
						req
							.respond(Response::empty(404).with_header(header_cors))
							.unwrap();
					}
				}
			}
		}
	});
}
