package com.exit66.jukebox.servlet;

import java.io.File;
import java.io.FileInputStream;
import java.io.IOException;
import java.io.PrintWriter;

import javax.servlet.ServletOutputStream;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

abstract class BaseServlet extends HttpServlet {
	private static final long serialVersionUID = 5434096963886472237L;

	protected String[] splitCommands(HttpServletRequest req) {
		if (req.getPathInfo() != null) {
			return req.getPathInfo().substring(1).split("/");
		} else {
			return new String[0];
		}
	}

	protected void sendJsonOutput(HttpServletResponse resp, String results) {
		PrintWriter out;
		try {
			resp.setContentType("application/json; charset=" + resp.getCharacterEncoding());
			out = resp.getWriter();
			out.print("{\"results\":");
			out.print(results);
			out.print(",\"success\":1}");
			out.close();
		} catch (IOException e) {
			e.printStackTrace();
		}
	}

	private String quote(String value) {
		return "\"" + value.replace("\"", "\\\"") + "\"";
	}

	protected void sendErrorOutput(HttpServletResponse resp, String message) {
		PrintWriter out;
		try {
			resp.setContentType("application/json; charset=" + resp.getCharacterEncoding());
			out = resp.getWriter();
			out.print("{\"message\":");
			out.print(quote(message));
			out.print(",\"success\":0}");
			out.close();
		} catch (IOException e) {
			e.printStackTrace();
		}
	}

	protected void sendSuccessOutput(HttpServletResponse resp) {
		PrintWriter out;
		try {
			resp.setContentType("application/json; charset=" + resp.getCharacterEncoding());
			out = resp.getWriter();
			out.print("{\"success\":1}");
			out.close();
		} catch (IOException e) {
			e.printStackTrace();
		}
	}

	protected void sendSuccessOutput(HttpServletResponse resp, String message) {
		PrintWriter out;
		try {
			resp.setContentType("application/json; charset=" + resp.getCharacterEncoding());
			out = resp.getWriter();
			out.print("{\"message\":");
			out.print(quote(message));
			out.print(",\"success\":1}");
			out.close();
		} catch (IOException e) {
			e.printStackTrace();
		}
	}
	
	protected void sendFileOutput(HttpServletResponse resp, File file, String mimeType) {
		try {
			FileInputStream input = new FileInputStream(file);
			ServletOutputStream out = resp.getOutputStream();
			
			resp.addHeader("Cache-Control", "max-age=7");
			resp.addHeader("Content-length", String.valueOf(file.length()));
			resp.addHeader("Content-type", mimeType);
			
			byte[] tmp = new byte[1024];
			int bytes;
			while ((bytes = input.read(tmp)) != -1) {
				out.write(tmp, 0, bytes);
			}
			out.close();
		} catch (IOException e) {
			e.printStackTrace();
		}
	}
}
