package com.exit66.jukebox.data;

/**
 * @author andyb
 *
 * To change this generated comment edit the template variable "typecomment":
 * Window>Preferences>Java>Templates.
 * To enable and disable the creation of type comments go to
 * Window>Preferences>Java>Code Generation.
 */

public abstract class BVDatabaseObject extends BVDatabase {
    
    public boolean fetch() {
        
        return true;
        
    }
    
    public void save() {
        
    }
    
    public void remove() {
        
    }
    
    public void toJson(StringBuffer sb) {
    	toJson(sb, 0);
    }
    
    public abstract void toJson(StringBuffer sb, int childLevel);
    
    private String quote(String value) {
    	return "\"" + value.replace("\"", "\\\"") + "\"";
    }
    protected void appendJsonElement(StringBuffer sb, String name, String value) {
    	if (value == null) {
    		value = "";
    	}
    	sb.append(quote(name.trim()));
    	sb.append(": ");
    	sb.append(quote(value.replace("\\", "\\\\")));
    }
    
    protected void appendJsonElement(StringBuffer sb, String name, int value) {
    	sb.append(quote(name.trim()));
    	sb.append(": ");
    	sb.append(value);
    }
    
    protected void appendJsonElement(StringBuffer sb, String name, BVDatabaseCollection value, int childLevel) {
    	sb.append(quote(name.trim()));
    	sb.append(": ");
    	sb.append(value.toJson(childLevel));
    }
    
    public String toJson() {
    	return toJson(0);
    }
    
    public String toJson(int childLevel) {
    	StringBuffer sb = new StringBuffer();
    	this.toJson(sb, childLevel);
    	return sb.toString();
    }
}
