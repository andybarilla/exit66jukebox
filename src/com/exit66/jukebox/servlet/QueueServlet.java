package com.exit66.jukebox.servlet;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.RequestQueue;
import com.exit66.jukebox.data.TrackCollection;

public class QueueServlet extends BaseServlet {
	
	/**
	 * 
	 */
	private static final long serialVersionUID = 5463001433917025196L;

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
			list.listQueue(req.getSession().getId(), start, count);
			sendJsonOutput(resp, list.toJson());
			return;
		} 
		sendErrorOutput(resp, "Command not found");
	}

	@Override
	protected void doPost(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		String[] commands = splitCommands(req);

		if (commands.length == 1) {
			if (req.getParameter("action").compareTo("remove") == 0) {
				try {
					RequestQueue queue = new RequestQueue();
					queue.removeRequest(req.getSession().getId(), Integer.parseInt(commands[0]));
					sendSuccessOutput(resp);
					return;
				} catch (NumberFormatException nfs) {
					sendErrorOutput(resp, "Invalid id");
					return;
				}
			}
		} else {
			if (req.getParameter("action").compareTo("clear") == 0) {
				try {
					RequestQueue queue = new RequestQueue();
					queue.clear(req.getSession().getId());
					sendSuccessOutput(resp);
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
