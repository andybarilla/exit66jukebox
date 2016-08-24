package com.exit66.jukebox.servlet;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.Library;
import com.exit66.jukebox.data.LibraryCollection;

public class LibraryServlet extends BaseServlet {
	private static final long serialVersionUID = 2081773601362968922L;

	@Override
	protected void doGet(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);

		if (commands.length == 0) {
			// list all the libraries
			LibraryCollection list = new LibraryCollection();
			list.list();
			sendJsonOutput(resp, list.toJson());
			return;
		} else if (commands.length == 1) {
			try {
				Library library = new Library();
				library.setLibraryID(Integer.parseInt(commands[0]));
				if (library.fetch()) {
					sendJsonOutput(resp, library.toJson());
				} else {
					sendErrorOutput(resp, "Invalid id");
				}
				return;
			} catch (NumberFormatException nfe) {
				sendErrorOutput(resp, "Invalid id");
				return;
			}
		}
		sendErrorOutput(resp, "Command not found");
	}

	@Override
	protected void doPost(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);

		if (commands.length == 0) {
			if (req.getParameter("action").compareTo("add") == 0) {
				addLibrary(resp, req.getParameter("path"));
				return;
			} else if (req.getParameter("action").compareTo("rescan") == 0) {
				Library.scanAll(true);
				sendSuccessOutput(resp);
				return;
			} else if (req.getParameter("action").compareTo("scan") == 0) {
				Library.scanAll();
				sendSuccessOutput(resp);
				return;
			}
		} else if (commands.length == 1) {
			if (req.getParameter("action").compareTo("delete") == 0) {
				try {
					Library library = new Library();
					library.setLibraryID(Integer.parseInt(commands[0]));
					if (library.fetch()) {
						library.remove();
						sendSuccessOutput(resp);
					} else {
						sendErrorOutput(resp, "Invalid id");
					}
					return;
				} catch (NumberFormatException nfs) {
					sendErrorOutput(resp, "Invalid id");
					return;
				}
			} else if (req.getParameter("action").compareTo("scan") == 0) {
				try {
					Library library = new Library();
					library.setLibraryID(Integer.parseInt(commands[0]));
					if (library.fetch()) {
						library.scan();
						sendSuccessOutput(resp);
					} else {
						sendErrorOutput(resp, "Invalid id");
					}
					return;
				} catch (NumberFormatException nfs) {
					sendErrorOutput(resp, "Invalid id");
					return;
				}
			}
		}
		sendErrorOutput(resp, "Command not found");
	}

	private void addLibrary(HttpServletResponse resp, String path) {
		Library library = new Library();
		library.setLibraryPath(path);
		library.save();
		library.scan(true);
		sendSuccessOutput(resp);
	}
}
