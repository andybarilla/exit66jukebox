package com.exit66.jukebox.servlet;

import java.io.IOException;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import com.exit66.jukebox.data.Stats;

public class StatsServlet extends BaseServlet {

	/**
	 * 
	 */
	private static final long serialVersionUID = 5303061268656049877L;

	protected void doGet(HttpServletRequest req, HttpServletResponse resp)
			throws ServletException, IOException {
		Stats stats = new Stats();
		sendJsonOutput(resp, stats.toJson());
	}
}
