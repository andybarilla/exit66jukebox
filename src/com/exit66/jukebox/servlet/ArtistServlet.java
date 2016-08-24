package com.exit66.jukebox.servlet;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.Artist;
import com.exit66.jukebox.data.ArtistCollection;
import com.exit66.jukebox.util.CoverImages;

public class ArtistServlet extends BaseServlet {
	private static final long serialVersionUID = -2652192654944763186L;

	@Override
	protected void doGet(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);

		if (commands.length == 0) {
			ArtistCollection list = new ArtistCollection();
			int start = 0;
			int count = 0;
			if (req.getParameter("offset") != null) {
				start = Integer.parseInt(req.getParameter("offset"));
			}
			if (req.getParameter("limit") != null) {
				count = Integer.parseInt(req.getParameter("limit"));
			}
			if (req.getParameter("startswith") != null) {
				list.list(req.getParameter("startswith") + "%", start, count);
			} else if (req.getParameter("search") != null) {
				list.list("%" + req.getParameter("search") + "%", start,
						count);
			} else {
				list.list(true, start, count);
			}
			sendJsonOutput(resp, list.toJson());
			return;
		} else if (commands.length == 1) {
			try {
				int artistId = Integer.parseInt(commands[0]);
				if ((req.getParameter("action") != null)
						&& (req.getParameter("action").compareTo("image") == 0)) {
					CoverImages images = new CoverImages();
					images.getArtistImage(req, resp, artistId);
					return;
				} else {
					Artist artist = new Artist();
					artist.setArtistID(artistId);
					artist.fetch();
					if (req.getParameter("track_detail") == null) {
						sendJsonOutput(resp, artist.toJson(1));
					} else {
						sendJsonOutput(resp, artist.toJson(2));
					}
					return;
				}
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
		sendErrorOutput(resp, "Command not found");
	}
}
