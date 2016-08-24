package com.exit66.jukebox.servlet;

import java.io.IOException;
import java.io.PrintWriter;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.Artist;
import com.exit66.jukebox.data.ArtistCollection;

public class AutoCompleteServlet extends HttpServlet {
	private static final long serialVersionUID = -2316029266078625813L;

	public void doGet(HttpServletRequest req, HttpServletResponse res)
			throws ServletException, IOException {
		doPost(req, res);
	}

	public void doPost(HttpServletRequest req, HttpServletResponse res)
			throws ServletException, IOException {
		PrintWriter out = res.getWriter();
		out.println("<ul>");

		if (req.getPathInfo().equals("/artistname"))
			processArtistName(req, out);

		out.println("</ul>");

		out.close();
	}

	protected void addItemToList(String item, PrintWriter out) {
		out.println("<li>" + item + "</li>");
	}

	protected void processArtistName(HttpServletRequest req, PrintWriter out) {
		if (req.getParameterMap().containsKey("value")) {
			ArtistCollection artistcol = new ArtistCollection();
			artistcol.list("%" + req.getParameter("value") + "%", 0, 50);
			while (!artistcol.isEOF()) {
				Artist artist = (Artist) artistcol.getCurrent();
				addItemToList(artist.getArtistName(), out);
				artistcol.moveNext();
			}
		}
	}

}
