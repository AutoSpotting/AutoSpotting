package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	autospotting "github.com/AutoSpotting/AutoSpotting/core"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/namsral/flag"
)

var as *autospotting.AutoSpotting
var conf autospotting.Config

// Version represents the build version being used
var Version = "number missing"

var eventFile string

func main() {

	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(Handler)
	} else if eventFile != "" {
		parseEvent, err := ioutil.ReadFile(eventFile)
		if err != nil {
			log.Fatal(err)
		}
		Handler(context.TODO(), parseEvent)
	} else {
		eventHandler(nil)
	}
}

func eventHandler(event *json.RawMessage) {

	log.Println("Starting autospotting agent, build", Version)
	log.Printf("Configuration flags: %#v", conf)

	as.EventHandler(event)
	log.Println("Execution completed, nothing left to do")
}

// this is the equivalent of a main for when running from Lambda, but on Lambda
// the runFromCronEvent() is executed within the handler function every time we have an event
func init() {

	var region string

	as = &autospotting.AutoSpotting{}

	if r := os.Getenv("AWS_REGION"); r != "" {
		region = r
	} else {
		region = endpoints.UsEast1RegionID
	}

	conf = autospotting.Config{
		LogFile:         os.Stdout,
		LogFlag:         log.Ldate | log.Ltime | log.Lshortfile,
		MainRegion:      region,
		SleepMultiplier: 1,
		Version:         Version,
		LicenseType:     os.Getenv("LICENSE"),
	}
	parseCommandLineFlags()

	as.Init(&conf)
}

// Handler implements the AWS Lambda handler interface
func Handler(ctx context.Context, rawEvent json.RawMessage) {
	eventHandler(&rawEvent)
}

func parseCommandLineFlags() {
	flag.StringVar(&conf.AllowedInstanceTypes, "allowed_instance_types", "",
		"\n\tIf specified, the spot instances will be searched only among these types.\n\tIf missing, any instance type is allowed.\n"+
			"\tAccepts a list of comma or whitespace separated instance types (supports globs).\n"+
			"\tExample: ./AutoSpotting -allowed_instance_types 'c5.*,c4.xlarge'\n")

	flag.StringVar(&conf.BiddingPolicy, "bidding_policy", autospotting.DefaultBiddingPolicy,
		"\n\tPolicy choice for spot bid. If set to 'normal', we bid at the on-demand price(times the multiplier).\n"+
			"\tIf set to 'aggressive', we bid at a percentage value above the spot price \n"+
			"\tconfigurable using the spot_price_buffer_percentage.\n")

	flag.StringVar(&conf.DisallowedInstanceTypes, "disallowed_instance_types", "",
		"\n\tIf specified, the spot instances will _never_ be of these types.\n"+
			"\tAccepts a list of comma or whitespace separated instance types (supports globs).\n"+
			"\tExample: ./AutoSpotting -disallowed_instance_types 't2.*,c4.xlarge'\n")

	flag.StringVar(&conf.InstanceTerminationMethod, "instance_termination_method", autospotting.DefaultInstanceTerminationMethod,
		"\n\tInstance termination method.  Must be one of '"+autospotting.DefaultInstanceTerminationMethod+"' (default),\n"+
			"\t or 'detach' (compatibility mode, not recommended)\n")

	flag.StringVar(&conf.TerminationNotificationAction, "termination_notification_action", autospotting.DefaultTerminationNotificationAction,
		"\n\tTermination Notification Action.\n"+
			"\tValid choices:\n"+
			"\t'"+autospotting.DefaultTerminationNotificationAction+
			"' (terminate if lifecyclehook else detach) | 'terminate' (lifecyclehook triggered)"+
			" | 'detach' (lifecyclehook not triggered)\n")

	flag.Int64Var(&conf.MinOnDemandNumber, "min_on_demand_number", autospotting.DefaultMinOnDemandValue,
		"\n\tNumber of on-demand nodes to be kept running in each of the groups.\n\t"+
			"Can be overridden on a per-group basis using the tag "+autospotting.OnDemandNumberLong+".\n")

	flag.Float64Var(&conf.MinOnDemandPercentage, "min_on_demand_percentage", 0.0,
		"\n\tPercentage of the total number of instances in each group to be kept on-demand\n\t"+
			"Can be overridden on a per-group basis using the tag "+autospotting.OnDemandPercentageTag+
			"\n\tIt is ignored if min_on_demand_number is also set.\n")

	flag.Float64Var(&conf.OnDemandPriceMultiplier, "on_demand_price_multiplier", 1.0,
		"\n\tMultiplier for the on-demand price. Numbers less than 1.0 are useful for volume discounts.\n"+
			"\tExample: ./AutoSpotting -on_demand_price_multiplier 0.6 will have the on-demand price "+
			"considered at 60% of the actual value.\n")

	flag.StringVar(&conf.Regions, "regions", "",
		"\n\tRegions where it should be activated (separated by comma or whitespace, also supports globs).\n"+
			"\tBy default it runs on all regions.\n"+
			"\tExample: ./AutoSpotting -regions 'eu-*,us-east-1'\n")

	flag.Float64Var(&conf.SpotPriceBufferPercentage, "spot_price_buffer_percentage", autospotting.DefaultSpotPriceBufferPercentage,
		"\n\tBid a given percentage above the current spot price.\n\tProtects the group from running spot"+
			"instances that got significantly more expensive than when they were initially launched\n"+
			"\tThe tag "+autospotting.SpotPriceBufferPercentageTag+" can be used to override this on a group level.\n"+
			"\tIf the bid exceeds the on-demand price, we place a bid at on-demand price itself.\n")

	flag.StringVar(&conf.SpotProductDescription, "spot_product_description", autospotting.DefaultSpotProductDescription,
		"\n\tThe Spot Product to use when looking up spot price history in the market.\n"+
			"\tValid choices: Linux/UNIX | SUSE Linux | Windows | Linux/UNIX (Amazon VPC) | \n"+
			"\tSUSE Linux (Amazon VPC) | Windows (Amazon VPC)\n\tDefault value: "+autospotting.DefaultSpotProductDescription+"\n")

	flag.StringVar(&conf.TagFilteringMode, "tag_filtering_mode", "opt-in", "\n\tControls the behavior of the tag_filters option.\n"+
		"\tValid choices: opt-in | opt-out\n\tDefault value: 'opt-in'\n\tExample: ./AutoSpotting --tag_filtering_mode opt-out\n")

	flag.StringVar(&conf.FilterByTags, "tag_filters", "", "\n\tSet of tags to filter the ASGs on.\n"+
		"\tDefault if no value is set will be the equivalent of -tag_filters 'spot-enabled=true'\n"+
		"\tIn case the tag_filtering_mode is set to opt-out, it defaults to 'spot-enabled=false'\n"+
		"\tExample: ./AutoSpotting --tag_filters 'spot-enabled=true,Environment=dev,Team=vision'\n")

	flag.StringVar(&conf.CronSchedule, "cron_schedule", "* *", "\n\tCron-like schedule in which to"+
		"\tperform(or not) spot replacement actions. Format: hour day-of-week\n"+
		"\tExample: ./AutoSpotting --cron_schedule '9-18 1-5' # workdays during the office hours \n")

	flag.StringVar(&conf.CronScheduleState, "cron_schedule_state", "on", "\n\tControls whether to take actions "+
		"inside or outside the schedule defined by cron_schedule. Allowed values: on|off\n"+
		"\tExample: ./AutoSpotting --cron_schedule_state='off' --cron_schedule '9-18 1-5'  # would only take action outside the defined schedule\n")

	flag.StringVar(&conf.LicenseType, "license", "evaluation", "\n\tControls the terms under which you use AutoSpotting"+
		"Allowed values: evaluation|I_am_supporting_it_on_Patreon|I_contributed_to_development_within_the_last_year|I_built_it_from_source_code\n"+
		"\tExample: ./AutoSpotting --license evaluation\n")

	flag.StringVar(&eventFile, "event_file", "", "\n\tJSON file containing event data, "+
		"used for locally simulating execution from Lambda. AutoSpotting now expects to be "+
		"triggered by events and won't do anything if no event is passed either as result of "+
		"AWS instance state notifications or simulated manually using this flag.\n")

	v := flag.Bool("version", false, "Print version number and exit.\n")
	flag.Parse()
	printVersion(v)
}

func printVersion(v *bool) {
	if *v {
		fmt.Println("AutoSpotting build:", Version)
		os.Exit(0)
	}
}
