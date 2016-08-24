package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.sql.DriverManager;

import com.exit66.jukebox.Options;

public class BVDatabase {

	private static boolean init = false;

	protected static Connection conn;

	/* private static Connection _conn;
	 private static int		_connectionCount = 0;
	 private static String	_connection; */

	public BVDatabase() {

		if (init == false) {

			/* _connection = Options.getDBDirectory() + "juke";
			 
			 try {
			 
			 Class.forName("org.hsqldb.jdbcDriver");
			 
			 }
			 catch (ClassNotFoundException e) {
			 
			 System.err.println(e);
			 
			 }
			 
			 try {
			 
			 _conn = DriverManager.getConnection("jdbc:hsqldb:" + _connection, "sa", "");
			 
			 }
			 catch (SQLException e) {
			 
			 System.err.println(e);
			 
			 }
			 
			 init = true; */

		}

	}

	public static boolean checkConnection() {
		if (conn == null) {
			try {

				Class.forName("org.hsqldb.jdbcDriver");

			} catch (ClassNotFoundException e) {
				System.err.println(e);
				return false;
			}

			try {

				conn = DriverManager.getConnection("jdbc:hsqldb:"
						+ Options.getDBDirectory() + "juke2", "sa", "");

			} catch (SQLException e) {

				System.err.println(e);
				System.err.println("A previous instance of Exit66 JukeBox is already running.");
				return false;

			}
		}
		return true;
	}

	public boolean openConnection() {
		return checkConnection();
		/* try {
		 
		 if (_connectionCount > 0) {
		 
		 if (_conn.isClosed() == true) {
		 
		 _connectionCount = 0;
		 
		 }
		 
		 }
		 
		 }
		 catch (SQLException e) {
		 
		 _connectionCount = 0;
		 
		 }
		 
		 _connectionCount++;
		 
		 /* if (_connectionCount == 1) {
		 
		 try {
		 
		 Class.forName("org.hsqldb.jdbcDriver");
		 
		 }
		 catch (ClassNotFoundException e) {
		 
		 System.err.println(e);
		 return false;
		 
		 }
		 
		 try {
		 
		 _conn = DriverManager.getConnection("jdbc:hsqldb:" + _connection, "sa", "");
		 
		 }
		 catch (SQLException e) {
		 
		 System.err.println(e);
		 return false;
		 
		 }
		 
		 } */

	}

	public boolean executeStatement(String statement) {

		try {

			Statement stmt = conn.createStatement();

			stmt.execute(statement);

		} catch (SQLException e) {

			System.err.println(e);

		}

		return true;

	}
	

	public boolean executeStatement(PreparedStatement ps) {

		try {

			ps.execute();

		} catch (SQLException e) {

			System.err.println(e);

		}

		return true;

	}

	public ResultSet retrieveData(String statement) {

		try {

			Statement stmt = conn.createStatement();

			ResultSet result = stmt.executeQuery(statement);

			return result;

		} catch (SQLException e) {

			System.err.println(e);

		} catch (NullPointerException ne) {

			System.err.println(ne);

		}

		return null;

	}
	
	public ResultSet retrieveData(PreparedStatement ps) {
		
		try {

			ResultSet result = ps.executeQuery();

			return result;

		} catch (SQLException e) {

			System.err.println(e);

		} catch (NullPointerException ne) {

			System.err.println(ne);

		}

		return null;
	}

	public boolean closeConnection() {

		/*
		 _connectionCount--;
		 
		 if (_connectionCount < 0) {
		 
		 _connectionCount = 0;
		 
		 }
		 
		 /* if (_connectionCount == 0) {
		 
		 try {
		 
		 _conn.close();
		 
		 }
		 catch (SQLException e) {
		 
		 return false;
		 
		 }
		 
		 } */

		return true;

	}

	public String Qts(String input) {

		if (input != null) {
			return "'" + input.replaceAll("'", "''") + "'";
		} else {
			return "null";
		}

	}

	public String noNull(String input) {

		if (input == null)

			return "";

		else

			return input;

	}
}
