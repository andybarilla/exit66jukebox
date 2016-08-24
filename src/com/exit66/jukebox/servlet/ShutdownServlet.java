package com.exit66.jukebox.servlet;

import java.io.IOException;
import java.io.PrintWriter;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import com.exit66.jukebox.WebServer;

public class ShutdownServlet extends HttpServlet {
	private static final long serialVersionUID = -1861240854702591669L;

	public void doGet(HttpServletRequest req, HttpServletResponse res)
			throws ServletException, IOException {
		PrintWriter out = res.getWriter();
		out.print("<html><head><title>Exit66 Jukebox</title></head><body>Exit66 Jukebox has gone to sleep.<br /><br />Jeep, jeep, jeep, jeep, jeep...</body></html>");
		out.close();
		WebServer.getWebServer().shutDown();
	}

}
