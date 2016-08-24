package com.exit66.jukebox.data;

import java.sql.ResultSet;
import java.sql.SQLException;

public class Stats extends BVDatabaseObject {

	@Override
	public void toJson(StringBuffer sb, int childLevel) {
		sb.append("{ ");
		boolean firstRecord = true;
		
		if (openConnection()) {
        	
            try {
            	ResultSet rs = retrieveData("SELECT 'Artists' AS table_name, COUNT(*) as total FROM artist " +
            			"UNION SELECT 'Albums', COUNT(*) FROM album UNION SELECT 'Songs', COUNT(*) FROM track");
                    
                while (rs.next()) {
                    if (!firstRecord) {
                    	sb.append(", ");
                    } else {
                    	firstRecord = false;
                    }
                    appendJsonElement(sb, rs.getString("table_name"), rs.getInt("total"));
                } 
                
            } catch (SQLException e) {
                
                System.err.println(e);
                
            }
            
            closeConnection();
        } 
		
		sb.append("}");
	}

}
