package top.openremotehub.orn;

import android.app.Activity;
import android.app.Instrumentation;
import android.net.Uri;
import android.os.Bundle;

/** Runs against Android's actual Uri implementation, without an external test framework. */
public final class CompatibilityChecks extends Instrumentation {
    @Override public void onCreate(Bundle arguments) { super.onCreate(arguments); start(); }
    @Override public void onStart() {
        Bundle result=new Bundle();
        try {
            String address="127.0.0.1:54321";
            Uri link=MainActivity.rustDeskURI(address);
            if (!link.toString().equals("rustdesk://connection/new/"+address)
                || !link.getEncodedPath().equals("/new/"+address)) {
                throw new AssertionError("RustDesk received an encoded connection address");
            }
            Uri previous=new Uri.Builder().scheme("rustdesk").authority("connection").appendPath("new").appendPath(address).build();
            if (!previous.getEncodedPath().contains("%3A")) throw new AssertionError("Regression fixture no longer reproduces colon encoding");
            result.putString("stream","RustDesk URI preserves the literal port colon. Previous URI reproduces %3A.\n");
            finish(Activity.RESULT_OK,result);
        } catch (Throwable error) {
            result.putString("stream",error.toString());
            finish(Activity.RESULT_CANCELED,result);
        }
    }
}
