use std::env;

fn main() {
	let libdir = env::var("PCAP_LIBDIR").unwrap_or_else(|_| "Lib".to_string());

	println!("cargo:rerun-if-env-changed=PCAP_LIBDIR");
	println!("cargo:rustc-link-search=native={}", libdir);
	println!("cargo:rustc-link-lib=Packet");
}