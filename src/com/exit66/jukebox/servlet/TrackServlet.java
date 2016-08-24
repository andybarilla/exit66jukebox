package com.exit66.jukebox.servlet;

import java.io.File;
import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.RequestQueue;
import com.exit66.jukebox.data.Track;
import com.exit66.jukebox.data.TrackCollection;
import com.exit66.jukebox.util.CoverImages;

public class TrackServlet extends BaseServlet {
	private static final long serialVersionUID = -5833483541285473852L;

	@Override
	protected void doGet(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);

		if (commands.length == 0) {
			TrackCollection list = new TrackCollection();
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
			if (commands[0].compareTo("queue") == 0) {
				RequestQueue queue = new RequestQueue();
				int nextTrackId = queue.getNextRequest(req.getSession().getId());
				if (nextTrackId != -1) {
					Track track = new Track();
					track.setTrackID(nextTrackId);
					track.fetch();
					sendJsonOutput(resp, track.toJson());
					return;
				} else {
					sendErrorOutput(resp, "No tracks");
					return;
				}
			} else {
				try {
					int trackId = Integer.parseInt(commands[0]);
					if (req.getParameter("action") != null) {
						if (req.getParameter("action").compareTo("image") == 0) {
							CoverImages images = new CoverImages();
							images.getTrackImage(req, resp, trackId);
							return;	
						} else if (req.getParameter("action").compareTo("stream") == 0) {
							Track track = new Track();
							track.setTrackID(trackId);
							track.fetch();
							sendFileOutput(resp, new File(track.getFullFileName()), "audio/mpeg");
							return;
						}				
					} else {
						Track track = new Track();
						track.setTrackID(trackId);
						track.fetch();
						sendJsonOutput(resp, track.toJson());
						return;
					}
				} catch (NumberFormatException nfe) {
					sendErrorOutput(resp, "Invalid id");
					return;
				}
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
					int ret = queue.requestTrack(req.getSession().getId(), Integer.parseInt(commands[0]));
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
