package com.exit66.jukebox.servlet;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.Album;
import com.exit66.jukebox.data.AlbumCollection;
import com.exit66.jukebox.data.RequestQueue;
import com.exit66.jukebox.util.CoverImages;

public class AlbumServlet extends BaseServlet {
	private static final long serialVersionUID = -2285543831608010997L;

	@Override
	protected void doGet(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);
		
		if (commands.length == 0) {
			AlbumCollection list = new AlbumCollection();
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
				list.list(start, count);
			}
			sendJsonOutput(resp, list.toJson());
			return;
		} else if (commands.length == 1) {
			try {
				int albumId = Integer.parseInt(commands[0]);
				if ((req.getParameter("action") != null)
						&& (req.getParameter("action").compareTo("image") == 0)) {
					CoverImages images = new CoverImages();
					images.getAlbumImage(req, resp, albumId);
					return;
				} else {
					Album album = new Album();
					album.setAlbumID(albumId);
					album.fetch();
					sendJsonOutput(resp, album.toJson(1));
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
		String[] commands = splitCommands(req);

		if (commands.length == 1) {
			if (req.getParameter("action").compareTo("request") == 0) {
				try {
					RequestQueue queue = new RequestQueue();
					int ret = queue.requestAlbum(req.getSession().getId(), Integer.parseInt(commands[0]));
					sendSuccessOutput(resp, RequestQueue.getQueueMessage(ret));
					return;
				} catch (NumberFormatException nfs) {
					sendErrorOutput(resp, "Invalid id");
					return;
				}
			}
		}
		sendErrorOutput(resp, "Command not found");
	}

}
