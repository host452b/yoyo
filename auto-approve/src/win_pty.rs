use std::io;

pub struct PtyChild {
    pub process: conpty::Process,
}

pub fn spawn(command: &[String], cols: u16, rows: u16) -> io::Result<PtyChild> {
    // Build a single command string with proper quoting for Windows
    let cmd_string = command
        .iter()
        .map(|arg| {
            if arg.contains(' ') || arg.contains('"') {
                format!("\"{}\"", arg.replace('"', "\\\""))
            } else {
                arg.clone()
            }
        })
        .collect::<Vec<_>>()
        .join(" ");

    let mut process = conpty::spawn(&cmd_string)
        .map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;

    // Set initial size (spawn doesn't accept dimensions in v0.7)
    process.resize(cols as i16, rows as i16)
        .map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;

    Ok(PtyChild { process })
}
